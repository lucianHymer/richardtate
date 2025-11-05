package main

import (
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"

	"github.com/pion/webrtc/v4"
	"github.com/yourusername/streaming-transcription/server/internal/api"
	"github.com/yourusername/streaming-transcription/server/internal/config"
	"github.com/yourusername/streaming-transcription/server/internal/logger"
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

	// Create WebRTC manager
	webrtcManager := webrtcmgr.New(log, iceServers)
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
