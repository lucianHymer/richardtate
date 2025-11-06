package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
	"github.com/lucianHymer/streaming-transcription/server/internal/transcription"
	"github.com/lucianHymer/streaming-transcription/shared/protocol"
)

// Manager handles WebRTC peer connections
type Manager struct {
	logger      *logger.ContextLogger
	peerConns   map[string]*PeerConnection
	peerConnsMu sync.RWMutex
	config      webrtc.Configuration

	// Factory config for creating pipelines
	whisperConfig    transcription.WhisperConfig
	rnnoiseModelPath string
	enableDebugWAV   bool
}

// PeerConnection represents a single WebRTC peer connection
type PeerConnection struct {
	ID          string
	pc          *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
	pipeline    *transcription.TranscriptionPipeline // Each peer has their own
	logger      *logger.ContextLogger
	onMessage   func(msg *protocol.Message)
}

// ManagerConfig contains configuration for creating pipelines
type ManagerConfig struct {
	WhisperConfig    transcription.WhisperConfig
	RNNoiseModelPath string
	EnableDebugWAV   bool
}

// New creates a new WebRTC manager
func New(log *logger.Logger, iceServers []webrtc.ICEServer, config ManagerConfig) *Manager {
	webrtcConfig := webrtc.Configuration{
		ICEServers: iceServers,
	}

	return &Manager{
		logger:           log.With("webrtc"),
		peerConns:        make(map[string]*PeerConnection),
		config:           webrtcConfig,
		whisperConfig:    config.WhisperConfig,
		rnnoiseModelPath: config.RNNoiseModelPath,
		enableDebugWAV:   config.EnableDebugWAV,
	}
}

// CreatePipelineForPeer creates a new pipeline with client-provided settings
func (m *Manager) CreatePipelineForPeer(peerID string, settings *protocol.ControlStartData) (*transcription.TranscriptionPipeline, error) {
	m.peerConnsMu.RLock()
	peer, exists := m.peerConns[peerID]
	m.peerConnsMu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("peer %s not found", peerID)
	}

	// Create pipeline config with client settings
	config := transcription.PipelineConfig{
		WhisperConfig:      m.whisperConfig,
		RNNoiseModelPath:   m.rnnoiseModelPath,
		VADEnergyThreshold: settings.VADEnergyThreshold,
		SilenceThreshold:   time.Duration(settings.SilenceThresholdMs) * time.Millisecond,
		MinChunkDuration:   time.Duration(settings.MinChunkDurationMs) * time.Millisecond,
		MaxChunkDuration:   time.Duration(settings.MaxChunkDurationMs) * time.Millisecond,
		EnableDebugWAV:     m.enableDebugWAV,
	}

	// Create pipeline
	pipeline, err := transcription.NewTranscriptionPipeline(config)
	if err != nil {
		return nil, fmt.Errorf("failed to create pipeline: %w", err)
	}

	// Store in peer connection
	peer.pipeline = pipeline
	m.logger.Info("Created pipeline for peer %s with VAD threshold %.0f", peerID, settings.VADEnergyThreshold)

	return pipeline, nil
}

// GetPeerPipeline returns the pipeline for a specific peer
func (m *Manager) GetPeerPipeline(peerID string) *transcription.TranscriptionPipeline {
	m.peerConnsMu.RLock()
	defer m.peerConnsMu.RUnlock()

	if peer, exists := m.peerConns[peerID]; exists {
		return peer.pipeline
	}
	return nil
}

// CreatePeerConnection creates a new peer connection
func (m *Manager) CreatePeerConnection(id string, onMessage func(msg *protocol.Message)) (*PeerConnection, error) {
	m.peerConnsMu.Lock()
	defer m.peerConnsMu.Unlock()

	// Check if peer already exists
	if _, exists := m.peerConns[id]; exists {
		return nil, fmt.Errorf("peer connection %s already exists", id)
	}

	pc, err := webrtc.NewPeerConnection(m.config)
	if err != nil {
		return nil, fmt.Errorf("failed to create peer connection: %w", err)
	}

	peer := &PeerConnection{
		ID:        id,
		pc:        pc,
		logger:    m.logger,
		onMessage: onMessage,
	}

	// Set up connection state handler
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		peer.logger.Info("Peer %s connection state: %s", id, state.String())

		if state == webrtc.PeerConnectionStateFailed ||
		   state == webrtc.PeerConnectionStateClosed {
			m.RemovePeerConnection(id)
		}
	})

	// Set up ICE connection state handler
	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		peer.logger.Debug("Peer %s ICE state: %s", id, state.String())
	})

	// Set up DataChannel handler (client will create the channel)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		peer.logger.Info("DataChannel '%s' opened by peer %s", dc.Label(), id)
		peer.dataChannel = dc

		// Set up message handler
		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			peer.handleMessage(msg.Data)
		})

		dc.OnOpen(func() {
			peer.logger.Info("DataChannel '%s' is open", dc.Label())
		})

		dc.OnClose(func() {
			peer.logger.Info("DataChannel '%s' closed", dc.Label())
		})

		dc.OnError(func(err error) {
			peer.logger.Error("DataChannel error: %v", err)
		})
	})

	m.peerConns[id] = peer
	m.logger.Info("Created peer connection for %s", id)

	return peer, nil
}

// RemovePeerConnection removes a peer connection
func (m *Manager) RemovePeerConnection(id string) {
	m.peerConnsMu.Lock()
	defer m.peerConnsMu.Unlock()

	if peer, exists := m.peerConns[id]; exists {
		// Clean up pipeline if it exists
		if peer.pipeline != nil {
			peer.pipeline.Stop()
			peer.pipeline.Close()
			m.logger.Info("Closed pipeline for peer %s", id)
		}

		if peer.pc != nil {
			peer.pc.Close()
		}
		delete(m.peerConns, id)
		m.logger.Info("Removed peer connection %s", id)
	}
}

// GetPeerConnection returns a peer connection by ID
func (m *Manager) GetPeerConnection(id string) (*PeerConnection, bool) {
	m.peerConnsMu.RLock()
	defer m.peerConnsMu.RUnlock()
	peer, exists := m.peerConns[id]
	return peer, exists
}

// CreateOffer creates a WebRTC offer
func (p *PeerConnection) CreateOffer() (string, error) {
	offer, err := p.pc.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create offer: %w", err)
	}

	if err := p.pc.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	offerJSON, err := json.Marshal(offer)
	if err != nil {
		return "", fmt.Errorf("failed to marshal offer: %w", err)
	}

	return string(offerJSON), nil
}

// CreateAnswer creates a WebRTC answer from an offer
func (p *PeerConnection) CreateAnswer(offerJSON string) (string, error) {
	var offer webrtc.SessionDescription
	if err := json.Unmarshal([]byte(offerJSON), &offer); err != nil {
		return "", fmt.Errorf("failed to unmarshal offer: %w", err)
	}

	if err := p.pc.SetRemoteDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set remote description: %w", err)
	}

	answer, err := p.pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	if err := p.pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	answerJSON, err := json.Marshal(answer)
	if err != nil {
		return "", fmt.Errorf("failed to marshal answer: %w", err)
	}

	return string(answerJSON), nil
}

// AddICECandidate adds an ICE candidate
func (p *PeerConnection) AddICECandidate(candidateJSON string) error {
	var candidate webrtc.ICECandidateInit
	if err := json.Unmarshal([]byte(candidateJSON), &candidate); err != nil {
		return fmt.Errorf("failed to unmarshal ICE candidate: %w", err)
	}

	if err := p.pc.AddICECandidate(candidate); err != nil {
		return fmt.Errorf("failed to add ICE candidate: %w", err)
	}

	return nil
}

// SendMessage sends a message over the DataChannel
func (p *PeerConnection) SendMessage(msg *protocol.Message) error {
	if p.dataChannel == nil {
		return fmt.Errorf("data channel not ready")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return p.dataChannel.Send(data)
}

// handleMessage handles incoming DataChannel messages
func (p *PeerConnection) handleMessage(data []byte) {
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		p.logger.Error("Failed to unmarshal message: %v", err)
		return
	}

	p.logger.Debug("Received message type: %s", msg.Type)

	if p.onMessage != nil {
		p.onMessage(&msg)
	}
}

// GatherICECandidates sets up ICE candidate gathering
func (p *PeerConnection) GatherICECandidates(onCandidate func(string)) {
	p.pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		candidateJSON, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			p.logger.Error("Failed to marshal ICE candidate: %v", err)
			return
		}

		onCandidate(string(candidateJSON))
	})
}

// Close closes the peer connection
func (p *PeerConnection) Close() error {
	if p.pc != nil {
		return p.pc.Close()
	}
	return nil
}
