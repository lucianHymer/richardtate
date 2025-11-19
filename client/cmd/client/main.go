package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/lucianHymer/streaming-transcription/client/internal/audio"
	"github.com/lucianHymer/streaming-transcription/client/internal/config"
	"github.com/lucianHymer/streaming-transcription/client/internal/debuglog"
	"github.com/lucianHymer/streaming-transcription/client/internal/ui"
	"github.com/lucianHymer/streaming-transcription/client/internal/webrtc"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"github.com/lucianHymer/streaming-transcription/shared/protocol"
)

// Global instances (set in main)
var (
	globalLog      *logger.Logger
	globalDebugLog *debuglog.Logger
	globalUI       *ui.UI
)

// Session state for tracking complete transcriptions
var (
	sessionMu        sync.Mutex
	sessionChunks    []string
	sessionStart     time.Time
	sessionRecording bool
)

// getDefaultConfigPath returns the XDG Base Directory compliant config path
func getDefaultConfigPath() string {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "client.yaml" // Fallback to current directory
	}
	return filepath.Join(homeDir, ".config", "richardtate", "client.yaml")
}

func main() {
	defaultConfigPath := getDefaultConfigPath()
	configPath := flag.String("config", defaultConfigPath, "Path to configuration file")
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
	globalLog = log

	// Initialize debug log
	debugLog, err := debuglog.New(cfg.Client.DebugLogPath)
	if err != nil {
		log.Fatal("Failed to create debug log: %v", err)
	}
	defer debugLog.Close()
	globalDebugLog = debugLog
	log.Info("Debug log initialized at: %s", cfg.Client.DebugLogPath)

	log.Info("Starting streaming transcription client (native UI)")

	// Create WebRTC client
	webrtcClient := webrtc.New(cfg.Server.URL+"/api/v1/stream/signal", cfg, log, handleDataChannelMessage)

	// Create audio capturer
	capturer, err := audio.New(20, cfg.Audio.DeviceName, log)
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
			if err := webrtcClient.SendAudioChunk(chunk.Data, chunk.SampleRate, chunk.Channels); err != nil {
				log.Error("Failed to send audio chunk: %v", err)
			} else {
				log.Debug("Sent audio chunk: seq=%d, size=%d bytes", chunk.SequenceID, len(chunk.Data))
			}
		}
		log.Info("Audio sending goroutine stopped")
	}()

	// Connect to server in background
	go func() {
		log.Info("Connecting to server...")
		if err := webrtcClient.Connect(); err != nil {
			log.Error("Failed to connect to server: %v", err)
			return
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
			log.Error("Failed to establish DataChannel connection within timeout")
			return
		}

		log.Info("DataChannel connected!")

		// Send a test ping
		if err := webrtcClient.SendPing(); err != nil {
			log.Error("Failed to send ping: %v", err)
		} else {
			log.Info("Ping sent successfully")
		}
	}()

	// Create native UI
	globalUI = ui.New(cfg, log)

	// Set recording handlers
	globalUI.SetHandlers(
		func() error {
			// Start recording
			log.Info("Start recording requested")

			// Initialize session tracking
			sessionMu.Lock()
			sessionChunks = []string{}
			sessionStart = time.Now()
			sessionRecording = true
			sessionMu.Unlock()

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
			// Stop recording
			log.Info("Stop recording requested")

			// Stop audio capture first
			if err := capturer.Stop(); err != nil {
				log.Error("Failed to stop audio capture: %v", err)
				return err
			}
			log.Info("Audio capture stopped")

			// Log complete session to debug log
			sessionMu.Lock()
			sessionRecording = false
			fullText := strings.Join(sessionChunks, " ")
			duration := time.Since(sessionStart).Seconds()
			sessionMu.Unlock()

			if fullText != "" {
				if err := globalDebugLog.LogComplete(fullText, duration); err != nil {
					log.Error("Failed to log complete session: %v", err)
				} else {
					log.Info("Session logged: %d chunks, %.1f seconds, %d chars",
						len(sessionChunks), duration, len(fullText))
				}
			}

			// Send control stop message to server
			if err := webrtcClient.SendControlStop(); err != nil {
				log.Error("Failed to send control stop: %v", err)
				return err
			}
			log.Info("Sent control stop to server")
			return nil
		},
	)

	log.Info("Starting native UI - press Ctrl+N to toggle recording, Ctrl+Alt+C for calibration")

	// Run UI on main thread (blocks forever)
	// This is required because macOS UI must run on main thread
	globalUI.Run()

	// Cleanup (won't reach here normally - UI runs forever)
	if err := capturer.Close(); err != nil {
		log.Error("Error closing audio capturer: %v", err)
	}
	audioWg.Wait()

	if err := webrtcClient.Close(); err != nil {
		log.Error("Error closing WebRTC client: %v", err)
	}

	log.Info("Client stopped")
}

// handleDataChannelMessage handles messages received from the server
func handleDataChannelMessage(msg *protocol.Message) {
	messageLog := globalLog.With("message")

	switch msg.Type {
	case protocol.MessageTypeControlPong:
		messageLog.Info("Received pong from server!")

	case protocol.MessageTypeTranscriptPartial:
		var transcript protocol.TranscriptData
		if err := json.Unmarshal(msg.Data, &transcript); err != nil {
			messageLog.Error("Failed to unmarshal partial transcript: %v", err)
			return
		}
		fmt.Printf("[partial] %s\n", transcript.Text)

		// Update UI
		if globalUI != nil {
			globalUI.AppendTranscription(transcript.Text)
		}

	case protocol.MessageTypeTranscriptFinal:
		var transcript protocol.TranscriptData
		if err := json.Unmarshal(msg.Data, &transcript); err != nil {
			messageLog.Error("Failed to unmarshal final transcript: %v", err)
			return
		}
		fmt.Printf("%s\n", transcript.Text)

		// Update UI
		if globalUI != nil {
			globalUI.AppendTranscription(transcript.Text)
		}

		// Log chunk to debug log
		if err := globalDebugLog.LogChunk(transcript.Text); err != nil {
			messageLog.Error("Failed to log chunk to debug log: %v", err)
		}

		// Track session chunks
		sessionMu.Lock()
		if sessionRecording {
			sessionChunks = append(sessionChunks, transcript.Text)
		}
		sessionMu.Unlock()

	default:
		messageLog.Debug("Received message type: %s", string(msg.Type))
	}
}
