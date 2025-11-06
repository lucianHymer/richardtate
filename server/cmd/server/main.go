package main

import (
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/lucianHymer/streaming-transcription/server/internal/api"
	"github.com/lucianHymer/streaming-transcription/server/internal/config"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"github.com/lucianHymer/streaming-transcription/server/internal/transcription"
	webrtcmgr "github.com/lucianHymer/streaming-transcription/server/internal/webrtc"
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
	logLevel := logger.LevelInfo
	if cfg.Server.LogLevel != "" {
		logLevel = logger.ParseLogLevel(cfg.Server.LogLevel)
	}

	logFormat := logger.FormatText
	if cfg.Server.LogFormat != "" {
		logFormat = logger.ParseOutputFormat(cfg.Server.LogFormat)
	}

	log := logger.NewWithConfig(logger.Config{
		Level:  logLevel,
		Format: logFormat,
		Output: os.Stdout,
	})
	log.Info("Starting streaming transcription server")
	log.Info("Config: bind_address=%s, log_level=%s, log_format=%s",
		cfg.Server.BindAddress, logLevel.String(),
		map[logger.OutputFormat]string{logger.FormatText: "text", logger.FormatJSON: "json"}[logFormat])

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
			Logger:    log,
		},
		RNNoiseModelPath: cfg.NoiseSuppression.ModelPath,
		SilenceThreshold:   time.Duration(cfg.VAD.SilenceThresholdMs) * time.Millisecond,
		MinChunkDuration:   time.Duration(cfg.VAD.MinChunkDurationMs) * time.Millisecond,
		MaxChunkDuration:   time.Duration(cfg.VAD.MaxChunkDurationMs) * time.Millisecond,
		VADEnergyThreshold: cfg.VAD.EnergyThreshold,
		ResultChannelSize:  10,
		EnableDebugWAV:     cfg.Transcription.EnableDebugWAV,
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
