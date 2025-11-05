package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/yourusername/streaming-transcription/client/internal/logger"
	"github.com/yourusername/streaming-transcription/shared/protocol"
)

// Client handles WebRTC connection to the server
type Client struct {
	serverURL     string
	logger        *logger.ContextLogger
	pc            *webrtc.PeerConnection
	dataChannel   *webrtc.DataChannel
	wsConn        *websocket.Conn
	onMessage     func(msg *protocol.Message)
	connected     bool
	connectedMu   sync.RWMutex
	sequenceID    uint64
	sequenceIDMu  sync.Mutex
}

// New creates a new WebRTC client
func New(serverURL string, log *logger.Logger, onMessage func(msg *protocol.Message)) *Client {
	return &Client{
		serverURL: serverURL,
		logger:    log.With("webrtc"),
		onMessage: onMessage,
	}
}

// Connect establishes WebRTC connection to the server
func (c *Client) Connect() error {
	c.logger.Info("Connecting to server at %s", c.serverURL)

	// Connect WebSocket for signaling
	wsConn, _, err := websocket.DefaultDialer.Dial(c.serverURL, nil)
	if err != nil {
		return fmt.Errorf("failed to connect WebSocket: %w", err)
	}
	c.wsConn = wsConn

	// Create peer connection
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			// Empty for localhost connections - ICE not needed
		},
	}

	pc, err := webrtc.NewPeerConnection(config)
	if err != nil {
		wsConn.Close()
		return fmt.Errorf("failed to create peer connection: %w", err)
	}
	c.pc = pc

	// Set up connection state handlers
	pc.OnConnectionStateChange(func(state webrtc.PeerConnectionState) {
		c.logger.Info("Connection state: %s", state.String())

		c.connectedMu.Lock()
		if state == webrtc.PeerConnectionStateConnected {
			c.connected = true
		} else if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateClosed {
			c.connected = false
		}
		c.connectedMu.Unlock()
	})

	pc.OnICEConnectionStateChange(func(state webrtc.ICEConnectionState) {
		c.logger.Debug("ICE connection state: %s", state.String())
	})

	// Set up ICE candidate handler
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}

		candidateJSON, err := json.Marshal(candidate.ToJSON())
		if err != nil {
			c.logger.Error("Failed to marshal ICE candidate: %v", err)
			return
		}

		msg := protocol.SignalingMessage{
			Type: "ice",
			Data: json.RawMessage(candidateJSON),
		}

		if err := c.wsConn.WriteJSON(msg); err != nil {
			c.logger.Error("Failed to send ICE candidate: %v", err)
		}
	})

	// Create DataChannel with reliable/ordered mode
	ordered := true
	dataChannel, err := pc.CreateDataChannel("audio", &webrtc.DataChannelInit{
		Ordered:        &ordered,
		MaxRetransmits: nil, // Unlimited retries = reliable mode
	})
	if err != nil {
		pc.Close()
		wsConn.Close()
		return fmt.Errorf("failed to create data channel: %w", err)
	}
	c.dataChannel = dataChannel

	// Set up DataChannel handlers
	dataChannel.OnOpen(func() {
		c.logger.Info("DataChannel opened")
		c.connectedMu.Lock()
		c.connected = true
		c.connectedMu.Unlock()
	})

	dataChannel.OnClose(func() {
		c.logger.Info("DataChannel closed")
		c.connectedMu.Lock()
		c.connected = false
		c.connectedMu.Unlock()
	})

	dataChannel.OnMessage(func(msg webrtc.DataChannelMessage) {
		c.handleMessage(msg.Data)
	})

	dataChannel.OnError(func(err error) {
		c.logger.Error("DataChannel error: %v", err)
	})

	// Create offer
	offer, err := pc.CreateOffer(nil)
	if err != nil {
		pc.Close()
		wsConn.Close()
		return fmt.Errorf("failed to create offer: %w", err)
	}

	// Set local description
	if err := pc.SetLocalDescription(offer); err != nil {
		pc.Close()
		wsConn.Close()
		return fmt.Errorf("failed to set local description: %w", err)
	}

	// Send offer to server
	offerJSON, err := json.Marshal(offer)
	if err != nil {
		pc.Close()
		wsConn.Close()
		return fmt.Errorf("failed to marshal offer: %w", err)
	}

	offerMsg := protocol.SignalingMessage{
		Type: "offer",
		Data: json.RawMessage(offerJSON),
	}

	if err := c.wsConn.WriteJSON(offerMsg); err != nil {
		pc.Close()
		wsConn.Close()
		return fmt.Errorf("failed to send offer: %w", err)
	}

	c.logger.Debug("Sent offer to server")

	// Start signaling message handler in goroutine
	go c.handleSignaling()

	c.logger.Info("WebRTC connection initiated")
	return nil
}

// handleSignaling processes signaling messages from the server
func (c *Client) handleSignaling() {
	for {
		var msg protocol.SignalingMessage
		if err := c.wsConn.ReadJSON(&msg); err != nil {
			c.logger.Debug("Signaling WebSocket closed: %v", err)
			break
		}

		c.logger.Debug("Received signaling message type: %s", msg.Type)

		switch msg.Type {
		case "answer":
			// Server sent answer
			var answer webrtc.SessionDescription
			if err := json.Unmarshal(msg.Data, &answer); err != nil {
				c.logger.Error("Failed to unmarshal answer: %v", err)
				continue
			}

			if err := c.pc.SetRemoteDescription(answer); err != nil {
				c.logger.Error("Failed to set remote description: %v", err)
				continue
			}

			c.logger.Debug("Set remote description (answer)")

		case "ice":
			// Server sent ICE candidate
			var candidate webrtc.ICECandidateInit
			if err := json.Unmarshal(msg.Data, &candidate); err != nil {
				c.logger.Error("Failed to unmarshal ICE candidate: %v", err)
				continue
			}

			if err := c.pc.AddICECandidate(candidate); err != nil {
				c.logger.Error("Failed to add ICE candidate: %v", err)
				continue
			}

			c.logger.Debug("Added ICE candidate")

		default:
			c.logger.Warn("Unknown signaling message type: %s", msg.Type)
		}
	}
}

// handleMessage handles incoming DataChannel messages
func (c *Client) handleMessage(data []byte) {
	var msg protocol.Message
	if err := json.Unmarshal(data, &msg); err != nil {
		c.logger.Error("Failed to unmarshal message: %v", err)
		return
	}

	c.logger.Debug("Received message type: %s", msg.Type)

	if c.onMessage != nil {
		c.onMessage(&msg)
	}
}

// SendMessage sends a message over the DataChannel
func (c *Client) SendMessage(msg *protocol.Message) error {
	c.connectedMu.RLock()
	connected := c.connected
	c.connectedMu.RUnlock()

	if !connected || c.dataChannel == nil {
		return fmt.Errorf("data channel not ready")
	}

	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	return c.dataChannel.Send(data)
}

// SendPing sends a ping message to test the connection
func (c *Client) SendPing() error {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeControlPing,
		Timestamp: time.Now().UnixMilli(),
	}
	return c.SendMessage(msg)
}

// SendAudioChunk sends an audio chunk to the server
func (c *Client) SendAudioChunk(data []byte, sampleRate, channels int) error {
	c.sequenceIDMu.Lock()
	seqID := c.sequenceID
	c.sequenceID++
	c.sequenceIDMu.Unlock()

	audioData := protocol.AudioChunkData{
		SampleRate: sampleRate,
		Channels:   channels,
		Data:       data,
		SequenceID: seqID,
	}

	audioJSON, err := json.Marshal(audioData)
	if err != nil {
		return fmt.Errorf("failed to marshal audio data: %w", err)
	}

	msg := &protocol.Message{
		Type:      protocol.MessageTypeAudioChunk,
		Timestamp: time.Now().UnixMilli(),
		Data:      json.RawMessage(audioJSON),
	}

	return c.SendMessage(msg)
}

// IsConnected returns whether the DataChannel is connected
func (c *Client) IsConnected() bool {
	c.connectedMu.RLock()
	defer c.connectedMu.RUnlock()
	return c.connected
}

// Close closes the WebRTC connection
func (c *Client) Close() error {
	c.logger.Info("Closing WebRTC connection")

	c.connectedMu.Lock()
	c.connected = false
	c.connectedMu.Unlock()

	if c.dataChannel != nil {
		c.dataChannel.Close()
	}

	if c.pc != nil {
		c.pc.Close()
	}

	if c.wsConn != nil {
		c.wsConn.Close()
	}

	return nil
}
