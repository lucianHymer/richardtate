package main

import (
	"errors"
	"flag"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/yourusername/streaming-transcription/client/internal/api"
	"github.com/yourusername/streaming-transcription/client/internal/config"
	"github.com/yourusername/streaming-transcription/client/internal/logger"
	"github.com/yourusername/streaming-transcription/client/internal/webrtc"
	"github.com/yourusername/streaming-transcription/shared/protocol"
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
	log, err := logger.New(cfg.Client.Debug, cfg.Client.DebugLogPath, cfg.Client.DebugLogMaxSize)
	if err != nil {
		panic(err)
	}
	defer log.Close()

	log.Info("Starting streaming transcription client")
	log.Info("Config: server_url=%s, api_bind_address=%s, debug=%v",
		cfg.Server.URL, cfg.Client.APIBindAddress, cfg.Client.Debug)

	// Create WebRTC client
	webrtcClient := webrtc.New(cfg.Server.URL+"/api/v1/stream/signal", log, handleDataChannelMessage)

	// Connect to server
	log.Info("Connecting to server...")
	if err := webrtcClient.Connect(); err != nil {
		log.Fatal("Failed to connect to server: %v", err)
	}

	// Wait for connection to establish
	log.Info("Waiting for DataChannel to open...")
	for i := 0; i < 100; i++ {
		if webrtcClient.IsConnected() {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	if !webrtcClient.IsConnected() {
		log.Fatal("Failed to establish DataChannel connection within timeout")
	}

	log.Info("DataChannel connected! Sending test ping...")

	// Send a test ping
	if err := webrtcClient.SendPing(); err != nil {
		log.Error("Failed to send ping: %v", err)
	} else {
		log.Info("Ping sent successfully")
	}

	// Create API server for control
	apiServer := api.New(cfg.Client.APIBindAddress, log)
	apiServer.SetHandlers(
		func() error {
			log.Info("Start recording requested")
			// TODO: Start audio capture
			return nil
		},
		func() error {
			log.Info("Stop recording requested")
			// TODO: Stop audio capture
			return nil
		},
	)

	// Start API server in goroutine
	go func() {
		log.Info("Starting control API on %s", cfg.Client.APIBindAddress)
		if err := apiServer.Start(); err != nil {
			log.Error("API server error: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	log.Info("Client running - press Ctrl+C to stop")
	<-sigChan

	log.Info("Shutting down...")

	// Clean up
	if err := apiServer.Stop(); err != nil {
		log.Error("Error stopping API server: %v", err)
	}

	if err := webrtcClient.Close(); err != nil {
		log.Error("Error closing WebRTC client: %v", err)
	}

	log.Info("Client stopped")
}

// handleDataChannelMessage handles messages received from the server
func handleDataChannelMessage(msg *protocol.Message) {
	// This will be called when we receive messages from the server
	// For now, just log them
	switch msg.Type {
	case protocol.MessageTypeControlPong:
		println("✓ Received pong from server!")

	case protocol.MessageTypeTranscriptPartial:
		var transcript protocol.TranscriptData
		// TODO: Unmarshal and display partial transcription
		_ = transcript

	case protocol.MessageTypeTranscriptFinal:
		var transcript protocol.TranscriptData
		// TODO: Unmarshal and display final transcription
		_ = transcript

	default:
		println("Received message type:", string(msg.Type))
	}
}
