package main

import (
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/yourusername/streaming-transcription/server/internal/api"
	"github.com/yourusername/streaming-transcription/server/internal/config"
	"github.com/yourusername/streaming-transcription/server/internal/logger"
	"github.com/yourusername/streaming-transcription/server/internal/transcription"
	webrtcmgr "github.com/yourusername/streaming-transcription/server/internal/webrtc"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		// Try default config if file doesn't exist
		if errors.Is(err, os.ErrNotExist) {
			cfg = config.Default()
		} else {
			panic(err)
		}
	}

	// Initialize logger
	log := logger.New(cfg.Server.Debug)
	log.Info("Starting streaming transcription server")
	log.Info("Config: bind_address=%s, debug=%v", cfg.Server.BindAddress, cfg.Server.Debug)

	// Convert ICE servers
	var iceServers []webrtc.ICEServer
	for _, ice := range cfg.WebRTC.ICEServers {
		iceServers = append(iceServers, webrtc.ICEServer{
			URLs:       ice.URLs,
			Username:   ice.Username,
			Credential: ice.Credential,
		})
	}

	// Initialize transcription pipeline
	pipelineConfig := transcription.PipelineConfig{
		WhisperConfig: transcription.WhisperConfig{
			ModelPath: cfg.Transcription.ModelPath,
			Language:  cfg.Transcription.Language,
			Threads:   uint(cfg.Transcription.Threads),
		},
		RNNoiseModelPath: cfg.NoiseSuppression.ModelPath,
		SilenceThreshold: time.Duration(cfg.VAD.SilenceThresholdMs) * time.Millisecond,
		MinChunkDuration: time.Duration(cfg.VAD.MinChunkDurationMs) * time.Millisecond,
		MaxChunkDuration: time.Duration(cfg.VAD.MaxChunkDurationMs) * time.Millisecond,
		VADEnergyThreshold: cfg.VAD.EnergyThreshold,
		ResultChannelSize: 10,
		EnableDebugWAV: cfg.Server.Debug, // Save debug WAV files when in debug mode
	}

	pipeline, err := transcription.NewTranscriptionPipeline(pipelineConfig)
	if err != nil {
		log.Fatal("Failed to initialize transcription pipeline: %v", err)
	}
	defer pipeline.Close()
	log.Info("Transcription pipeline initialized with model: %s", cfg.Transcription.ModelPath)

	// Create WebRTC manager
	webrtcManager := webrtcmgr.New(log, iceServers, pipeline)
	log.Info("WebRTC manager initialized with %d ICE servers", len(iceServers))

	// Create API server
	apiServer := api.New(cfg.Server.BindAddress, log, webrtcManager)

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		if err := apiServer.Start(); err != nil {
			errChan <- err
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		log.Fatal("Server error: %v", err)
	case sig := <-sigChan:
		log.Info("Received signal %v, shutting down...", sig)
		if err := apiServer.Stop(); err != nil {
			log.Error("Error stopping server: %v", err)
		}
	}

	log.Info("Server stopped")
}
