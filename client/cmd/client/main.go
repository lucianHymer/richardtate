package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/lucianHymer/streaming-transcription/client/internal/api"
	"github.com/lucianHymer/streaming-transcription/client/internal/audio"
	"github.com/lucianHymer/streaming-transcription/client/internal/calibrate"
	"github.com/lucianHymer/streaming-transcription/client/internal/config"
	"github.com/lucianHymer/streaming-transcription/client/internal/webrtc"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"github.com/lucianHymer/streaming-transcription/shared/protocol"
)

func main() {
	configPath := flag.String("config", "config.yaml", "Path to configuration file")
	calibrateMode := flag.Bool("calibrate", false, "Run VAD calibration wizard")
	autoSave := flag.Bool("yes", false, "Auto-save calibration results without prompting")
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
	log := logger.New(cfg.Client.Debug)
	globalLog = log // Set global logger for message handler

	// Run calibration wizard if --calibrate flag is set
	if *calibrateMode {
		wizard, err := calibrate.NewWizard(cfg, log)
		if err != nil {
			log.Fatal("Failed to create calibration wizard: %v", err)
		}

		if err := wizard.Run(*configPath, *autoSave); err != nil {
			log.Fatal("Calibration failed: %v", err)
		}

		return
	}

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

	// Create audio capturer
	capturer, err := audio.New(20, cfg.Audio.DeviceName, log) // Buffer up to 20 chunks (4 seconds at 200ms/chunk)
	if err != nil {
		log.Fatal("Failed to create audio capturer: %v", err)
	}
	defer capturer.Close()

	// Goroutine to send audio chunks to server
	var audioWg sync.WaitGroup
	audioWg.Add(1)
	go func() {
		defer audioWg.Done()
		for chunk := range capturer.Chunks() {
			// Send raw PCM data via WebRTC
			// SendAudioChunk will handle the JSON marshaling
			if err := webrtcClient.SendAudioChunk(chunk.Data, chunk.SampleRate, chunk.Channels); err != nil {
				log.Error("Failed to send audio chunk: %v", err)
			} else {
				log.Debug("Sent audio chunk: seq=%d, size=%d bytes", chunk.SequenceID, len(chunk.Data))
			}
		}
		log.Info("Audio sending goroutine stopped")
	}()

	// Create API server for control
	apiServer := api.New(cfg.Client.APIBindAddress, log)
	apiServer.SetHandlers(
		func() error {
			log.Info("Start recording requested")

			// Send control start message to server
			if err := webrtcClient.SendControlStart(); err != nil {
				log.Error("Failed to send control start: %v", err)
				return err
			}
			log.Info("Sent control start to server")

			// Start audio capture
			if err := capturer.Start(); err != nil {
				log.Error("Failed to start audio capture: %v", err)
				return err
			}
			log.Info("Audio capture started")
			return nil
		},
		func() error {
			log.Info("Stop recording requested")

			// Stop audio capture first
			if err := capturer.Stop(); err != nil {
				log.Error("Failed to stop audio capture: %v", err)
				return err
			}
			log.Info("Audio capture stopped")

			// Save client-side recording for debugging
			// TODO: Remove this debug code after fixing audio issue
			log.Info("Client-side recording saved for debugging (not implemented yet)")

			// Send control stop message to server
			if err := webrtcClient.SendControlStop(); err != nil {
				log.Error("Failed to send control stop: %v", err)
				return err
			}
			log.Info("Sent control stop to server")
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

	// Stop audio capture (this closes the chunks channel)
	if err := capturer.Close(); err != nil {
		log.Error("Error closing audio capturer: %v", err)
	}

	// Wait for audio goroutine to finish
	log.Info("Waiting for audio goroutine to finish...")
	audioWg.Wait()

	if err := webrtcClient.Close(); err != nil {
		log.Error("Error closing WebRTC client: %v", err)
	}

	log.Info("Client stopped")
}

// Global logger for message handler (set in main)
var globalLog *logger.Logger

// handleDataChannelMessage handles messages received from the server
func handleDataChannelMessage(msg *protocol.Message) {
	messageLog := globalLog.With("message")

	// This will be called when we receive messages from the server
	switch msg.Type {
	case protocol.MessageTypeControlPong:
		messageLog.Info("✓ Received pong from server!")

	case protocol.MessageTypeTranscriptPartial:
		var transcript protocol.TranscriptData
		if err := json.Unmarshal(msg.Data, &transcript); err != nil {
			messageLog.Error("Failed to unmarshal partial transcript: %v", err)
			return
		}
		fmt.Printf("📝 [partial] %s\n", transcript.Text)

	case protocol.MessageTypeTranscriptFinal:
		var transcript protocol.TranscriptData
		if err := json.Unmarshal(msg.Data, &transcript); err != nil {
			messageLog.Error("Failed to unmarshal final transcript: %v", err)
			return
		}
		fmt.Printf("✅ %s\n", transcript.Text)

	default:
		messageLog.Debug("Received message type: %s", string(msg.Type))
	}
}
