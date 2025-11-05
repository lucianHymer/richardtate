package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/pion/webrtc/v4"
	"github.com/yourusername/streaming-transcription/server/internal/logger"
	"github.com/yourusername/streaming-transcription/shared/protocol"
)

// Manager handles WebRTC peer connections
type Manager struct {
	logger      *logger.ContextLogger
	peerConns   map[string]*PeerConnection
	peerConnsMu sync.RWMutex
	config      webrtc.Configuration
}

// PeerConnection represents a single WebRTC peer connection
type PeerConnection struct {
	ID         string
	pc         *webrtc.PeerConnection
	dataChannel *webrtc.DataChannel
	logger     *logger.ContextLogger
	onMessage  func(msg *protocol.Message)
}

// New creates a new WebRTC manager
func New(log *logger.Logger, iceServers []webrtc.ICEServer) *Manager {
	config := webrtc.Configuration{
		ICEServers: iceServers,
	}

	return &Manager{
		logger:    log.With("webrtc"),
		peerConns: make(map[string]*PeerConnection),
		config:    config,
	}
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
