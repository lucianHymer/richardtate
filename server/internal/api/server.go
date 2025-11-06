package api

import (
	"encoding/json"
	"math"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"github.com/lucianHymer/streaming-transcription/server/internal/transcription"
	"github.com/lucianHymer/streaming-transcription/server/internal/webrtc"
	"github.com/lucianHymer/streaming-transcription/shared/protocol"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now (can be restricted later)
		return true
	},
}

// Server handles HTTP and WebSocket requests
type Server struct {
	bindAddr      string
	logger        *logger.ContextLogger
	server        *http.Server
	webrtcManager *webrtc.Manager
}

// New creates a new API server
func New(bindAddr string, log *logger.Logger, webrtcMgr *webrtc.Manager) *Server {
	return &Server{
		bindAddr:      bindAddr,
		logger:        log.With("api"),
		webrtcManager: webrtcMgr,
	}
}

// Start starts the HTTP server
func (s *Server) Start() error {
	mux := http.NewServeMux()

	// Register handlers
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/api/v1/stream/signal", s.handleSignaling)
	mux.HandleFunc("/api/v1/analyze-audio", s.handleAnalyzeAudio)

	s.server = &http.Server{
		Addr:         s.bindAddr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("Starting HTTP server on %s", s.bindAddr)
	return s.server.ListenAndServe()
}

// Stop gracefully stops the server
func (s *Server) Stop() error {
	if s.server != nil {
		return s.server.Close()
	}
	return nil
}

// handleHealth handles health check requests
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().Unix(),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(response)
}

// handleSignaling handles WebRTC signaling over WebSocket
func (s *Server) handleSignaling(w http.ResponseWriter, r *http.Request) {
	// Upgrade to WebSocket
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Error("Failed to upgrade to WebSocket: %v", err)
		return
	}
	defer conn.Close()

	// Generate peer ID
	peerID := uuid.New().String()
	s.logger.Info("New signaling connection from peer %s", peerID)

	// Declare peer variable first for closure
	var peer *webrtc.PeerConnection

	// Create peer connection
	peer, err = s.webrtcManager.CreatePeerConnection(peerID, func(msg *protocol.Message) {
		s.handleDataChannelMessage(peerID, peer, msg)
	})
	if err != nil {
		s.logger.Error("Failed to create peer connection: %v", err)
		return
	}
	defer s.webrtcManager.RemovePeerConnection(peerID)

	// Set up ICE candidate handler
	peer.GatherICECandidates(func(candidateJSON string) {
		msg := protocol.SignalingMessage{
			Type: "ice",
			Data: json.RawMessage(candidateJSON),
		}
		if err := conn.WriteJSON(msg); err != nil {
			s.logger.Error("Failed to send ICE candidate: %v", err)
		}
	})

	// Handle signaling messages
	for {
		var msg protocol.SignalingMessage
		if err := conn.ReadJSON(&msg); err != nil {
			s.logger.Debug("WebSocket read error (peer %s): %v", peerID, err)
			break
		}

		s.logger.Debug("Received signaling message type: %s from peer %s", msg.Type, peerID)

		switch msg.Type {
		case "offer":
			// Client sent an offer, create an answer
			answer, err := peer.CreateAnswer(string(msg.Data))
			if err != nil {
				s.logger.Error("Failed to create answer: %v", err)
				continue
			}

			response := protocol.SignalingMessage{
				Type: "answer",
				Data: json.RawMessage(answer),
			}
			if err := conn.WriteJSON(response); err != nil {
				s.logger.Error("Failed to send answer: %v", err)
			}

		case "ice":
			// Client sent an ICE candidate
			if err := peer.AddICECandidate(string(msg.Data)); err != nil {
				s.logger.Error("Failed to add ICE candidate: %v", err)
			}

		default:
			s.logger.Warn("Unknown signaling message type: %s", msg.Type)
		}
	}

	s.logger.Info("Signaling connection closed for peer %s", peerID)
}

// handleDataChannelMessage handles messages received over the DataChannel
func (s *Server) handleDataChannelMessage(peerID string, peer *webrtc.PeerConnection, msg *protocol.Message) {
	switch msg.Type {
	case protocol.MessageTypeControlPing:
		s.logger.Debug("Received ping from peer %s", peerID)

		// Send pong response
		pongMsg := &protocol.Message{
			Type:      protocol.MessageTypeControlPong,
			Timestamp: time.Now().UnixMilli(),
		}
		if err := peer.SendMessage(pongMsg); err != nil {
			s.logger.Error("Failed to send pong: %v", err)
		} else {
			s.logger.Debug("Sent pong to peer %s", peerID)
		}

	case protocol.MessageTypeAudioChunk:
		// Parse audio chunk
		var audioData protocol.AudioChunkData
		if err := json.Unmarshal(msg.Data, &audioData); err != nil {
			s.logger.Error("Failed to unmarshal audio chunk: %v", err)
			return
		}

		s.logger.Debug("Received audio chunk: seq=%d, size=%d bytes",
			audioData.SequenceID, len(audioData.Data))

		// Debug first chunk to see what we actually got
		if audioData.SequenceID == 0 && len(audioData.Data) >= 20 {
			s.logger.Debug("First chunk, first 20 bytes (hex): %x", audioData.Data[:20])
			s.logger.Debug("First chunk, first 5 samples (int16): %d %d %d %d %d",
				int16(audioData.Data[0])|int16(audioData.Data[1])<<8,
				int16(audioData.Data[2])|int16(audioData.Data[3])<<8,
				int16(audioData.Data[4])|int16(audioData.Data[5])<<8,
				int16(audioData.Data[6])|int16(audioData.Data[7])<<8,
				int16(audioData.Data[8])|int16(audioData.Data[9])<<8)
		}

		// Pass to this peer's transcription pipeline
		pipeline := s.webrtcManager.GetPeerPipeline(peerID)
		if pipeline != nil && pipeline.IsActive() {
			if err := pipeline.ProcessChunk(audioData.Data, msg.Timestamp); err != nil {
				s.logger.Error("Failed to process audio chunk: %v", err)
			}
		} else {
			s.logger.Debug("No active pipeline for peer %s, dropping audio chunk", peerID)
		}

	case protocol.MessageTypeControlStart:
		s.logger.Info("Received start command from peer %s", peerID)

		// Parse client settings from message data
		var controlData protocol.ControlStartData
		if msg.Data != nil {
			if err := json.Unmarshal(msg.Data, &controlData); err != nil {
				s.logger.Error("Failed to parse control start data: %v", err)
				return
			}
			s.logger.Info("Client settings: VAD=%.0f, Silence=%dms, Min=%dms, Max=%dms",
				controlData.VADEnergyThreshold,
				controlData.SilenceThresholdMs,
				controlData.MinChunkDurationMs,
				controlData.MaxChunkDurationMs)
		} else {
			s.logger.Warn("No settings provided in control.start, using defaults")
			// Use default values if not provided
			controlData = protocol.ControlStartData{
				VADEnergyThreshold: 500.0,
				SilenceThresholdMs: 1000,
				MinChunkDurationMs: 500,
				MaxChunkDurationMs: 30000,
			}
		}

		// Create pipeline with client settings
		pipeline, err := s.webrtcManager.CreatePipelineForPeer(peerID, &controlData)
		if err != nil {
			s.logger.Error("Failed to create pipeline: %v", err)
			return
		}

		// Start the pipeline
		if err := pipeline.Start(); err != nil {
			s.logger.Error("Failed to start pipeline: %v", err)
		} else {
			s.logger.Info("Transcription pipeline started for peer %s", peerID)

			// Start result sender goroutine
			go s.sendTranscriptionResults(peerID, peer, pipeline)
		}

	case protocol.MessageTypeControlStop:
		s.logger.Info("Received stop command from peer %s", peerID)

		// Stop transcription pipeline for this peer
		pipeline := s.webrtcManager.GetPeerPipeline(peerID)
		if pipeline != nil {
			if err := pipeline.Stop(); err != nil {
				s.logger.Error("Failed to stop pipeline: %v", err)
			} else {
				s.logger.Info("Transcription pipeline stopped for peer %s", peerID)
			}
		}

	default:
		s.logger.Warn("Unknown message type: %s", msg.Type)
	}
}

// sendTranscriptionResults reads from the pipeline results and sends them to the client
func (s *Server) sendTranscriptionResults(peerID string, peer *webrtc.PeerConnection, pipeline *transcription.TranscriptionPipeline) {
	s.logger.Info("Starting transcription result sender for peer %s", peerID)

	for result := range pipeline.Results() {
		// Check if there was an error
		if result.Error != nil {
			s.logger.Error("Transcription error: %v", result.Error)
			continue
		}

		// Skip empty transcriptions
		if result.Text == "" {
			continue
		}

		s.logger.Info("Transcription result: %q", result.Text)

		// Create transcription message
		transcriptData := protocol.TranscriptData{
			Text:    result.Text,
			IsFinal: true,
		}

		transcriptJSON, err := json.Marshal(transcriptData)
		if err != nil {
			s.logger.Error("Failed to marshal transcript data: %v", err)
			continue
		}

		msg := &protocol.Message{
			Type:      protocol.MessageTypeTranscriptFinal,
			Timestamp: result.Timestamp,
			Data:      transcriptJSON,
		}

		// Send to client
		if err := peer.SendMessage(msg); err != nil {
			s.logger.Error("Failed to send transcription to peer %s: %v", peerID, err)
			break
		}

		s.logger.Debug("Sent transcription to peer %s: %q", peerID, result.Text)
	}

	s.logger.Info("Transcription result sender stopped for peer %s", peerID)
}

// handleAnalyzeAudio analyzes audio samples and returns energy statistics
func (s *Server) handleAnalyzeAudio(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Parse request body
	var request struct {
		Audio []byte `json:"audio"` // PCM int16 audio data
	}

	if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
		s.logger.Error("Failed to decode analyze-audio request: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if len(request.Audio) == 0 {
		http.Error(w, "No audio data provided", http.StatusBadRequest)
		return
	}

	// Convert byte array to int16 samples
	if len(request.Audio)%2 != 0 {
		http.Error(w, "Audio data must be even number of bytes (int16 samples)", http.StatusBadRequest)
		return
	}

	samples := make([]int16, len(request.Audio)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(request.Audio[i*2]) | int16(request.Audio[i*2+1])<<8
	}

	// Apply RNNoise if available (matches production pipeline)
	processedSamples := samples
	pipeline := s.webrtcManager.GetPipeline()
	if pipeline != nil {
		rnnoise := pipeline.GetRNNoise()
		if rnnoise != nil {
			var err error
			processedSamples, err = rnnoise.ProcessChunk(samples)
			if err != nil {
				s.logger.Warn("RNNoise processing failed, using raw audio: %v", err)
				processedSamples = samples
			} else {
				s.logger.Debug("Applied RNNoise to calibration audio (%d samples)", len(processedSamples))
			}
		}
	}

	// Calculate statistics on processed audio
	stats := calculateAudioStatistics(processedSamples)

	s.logger.Info("Analyzed %d samples: min=%.1f, max=%.1f, avg=%.1f, p5=%.1f, p95=%.1f",
		stats.SampleCount, stats.Min, stats.Max, stats.Avg, stats.P5, stats.P95)

	// Return statistics
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(stats)
}

// AudioStatistics holds energy statistics for audio samples
type AudioStatistics struct {
	Min         float64 `json:"min"`
	Max         float64 `json:"max"`
	Avg         float64 `json:"avg"`
	P5          float64 `json:"p5"`  // 5th percentile
	P95         float64 `json:"p95"` // 95th percentile
	SampleCount int     `json:"sample_count"`
}

// calculateAudioStatistics computes energy statistics for audio samples
func calculateAudioStatistics(samples []int16) AudioStatistics {
	if len(samples) == 0 {
		return AudioStatistics{}
	}

	// Use 10ms frames (160 samples at 16kHz) for energy calculation
	// This matches the VAD implementation
	frameSize := 160
	var energies []float64

	for i := 0; i+frameSize <= len(samples); i += frameSize {
		frame := samples[i : i+frameSize]
		energy := calculateFrameEnergy(frame)
		energies = append(energies, energy)
	}

	if len(energies) == 0 {
		return AudioStatistics{}
	}

	// Calculate statistics
	var sum, min, max float64
	min = energies[0]
	max = energies[0]

	for _, e := range energies {
		sum += e
		if e < min {
			min = e
		}
		if e > max {
			max = e
		}
	}

	avg := sum / float64(len(energies))

	// Calculate percentiles (simple sort-based approach)
	sortedEnergies := make([]float64, len(energies))
	copy(sortedEnergies, energies)

	// Simple bubble sort (fine for small arrays)
	for i := 0; i < len(sortedEnergies); i++ {
		for j := i + 1; j < len(sortedEnergies); j++ {
			if sortedEnergies[i] > sortedEnergies[j] {
				sortedEnergies[i], sortedEnergies[j] = sortedEnergies[j], sortedEnergies[i]
			}
		}
	}

	p5Index := int(float64(len(sortedEnergies)) * 0.05)
	p95Index := int(float64(len(sortedEnergies)) * 0.95)
	if p95Index >= len(sortedEnergies) {
		p95Index = len(sortedEnergies) - 1
	}

	return AudioStatistics{
		Min:         min,
		Max:         max,
		Avg:         avg,
		P5:          sortedEnergies[p5Index],
		P95:         sortedEnergies[p95Index],
		SampleCount: len(samples),
	}
}

// calculateFrameEnergy computes RMS energy for a frame (matches VAD implementation)
func calculateFrameEnergy(samples []int16) float64 {
	if len(samples) == 0 {
		return 0
	}

	var sumSquares float64
	for _, sample := range samples {
		val := float64(sample)
		sumSquares += val * val
	}

	// IMPORTANT: Take square root for RMS (Root Mean Square)
	// This matches vad.go:calculateEnergy() exactly
	rms := math.Sqrt(sumSquares / float64(len(samples)))
	return rms
}
