package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/yourusername/streaming-transcription/server/internal/logger"
	"github.com/yourusername/streaming-transcription/server/internal/transcription"
	"github.com/yourusername/streaming-transcription/server/internal/webrtc"
	"github.com/yourusername/streaming-transcription/shared/protocol"
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

		// Pass to transcription pipeline
		pipeline := s.webrtcManager.GetPipeline()
		if pipeline != nil && pipeline.IsActive() {
			if err := pipeline.ProcessChunk(audioData.Data, msg.Timestamp); err != nil {
				s.logger.Error("Failed to process audio chunk: %v", err)
			}
		}

	case protocol.MessageTypeControlStart:
		s.logger.Info("Received start command from peer %s", peerID)

		// Start transcription pipeline
		pipeline := s.webrtcManager.GetPipeline()
		if pipeline != nil {
			if err := pipeline.Start(); err != nil {
				s.logger.Error("Failed to start pipeline: %v", err)
			} else {
				s.logger.Info("Transcription pipeline started for peer %s", peerID)

				// Start result sender goroutine
				go s.sendTranscriptionResults(peerID, peer, pipeline)
			}
		}

	case protocol.MessageTypeControlStop:
		s.logger.Info("Received stop command from peer %s", peerID)

		// Stop transcription pipeline
		pipeline := s.webrtcManager.GetPipeline()
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
