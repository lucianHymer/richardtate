package webrtc

import (
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/pion/webrtc/v4"
	"github.com/lucianHymer/streaming-transcription/client/internal/logger"
	"github.com/lucianHymer/streaming-transcription/shared/protocol"
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

	// Reconnection state
	reconnecting         bool
	reconnectingMu       sync.RWMutex
	reconnectAttempts    int
	maxReconnectAttempts int
	reconnectBaseDelay   time.Duration
	stopReconnect        chan struct{}

	// Audio chunk buffering during reconnection
	chunkBuffer     []bufferedChunk
	chunkBufferMu   sync.Mutex
	maxBufferSize   int
	droppedChunks   uint64

	// Connection state callback
	onConnectionStateChange func(connected bool, reconnecting bool)

	// Prevent multiple reconnection attempts
	reconnectOnce sync.Once
}

type bufferedChunk struct {
	data       []byte
	sampleRate int
	channels   int
	sequenceID uint64
	timestamp  int64
}

// New creates a new WebRTC client
func New(serverURL string, log *logger.Logger, onMessage func(msg *protocol.Message)) *Client {
	return &Client{
		serverURL:            serverURL,
		logger:               log.With("webrtc"),
		onMessage:            onMessage,
		maxReconnectAttempts: 10,
		reconnectBaseDelay:   time.Second,
		maxBufferSize:        100, // Buffer up to 100 chunks (20 seconds at 200ms/chunk)
		stopReconnect:        make(chan struct{}),
	}
}

// SetConnectionStateCallback sets a callback for connection state changes
func (c *Client) SetConnectionStateCallback(callback func(connected bool, reconnecting bool)) {
	c.onConnectionStateChange = callback
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
		previouslyConnected := c.connected

		if state == webrtc.PeerConnectionStateConnected {
			c.connected = true
			c.connectedMu.Unlock()

			// Reset reconnection state on successful connection
			c.reconnectingMu.Lock()
			if c.reconnecting {
				c.logger.Info("Reconnection successful! Flushing buffered chunks...")
				c.reconnecting = false
				c.reconnectAttempts = 0
				c.reconnectingMu.Unlock()

				// Flush any buffered chunks
				go c.flushBuffer()
			} else {
				c.reconnectingMu.Unlock()
			}

			// Notify callback
			if c.onConnectionStateChange != nil {
				c.onConnectionStateChange(true, false)
			}

		} else if state == webrtc.PeerConnectionStateFailed ||
			state == webrtc.PeerConnectionStateDisconnected {
			c.connected = false
			c.connectedMu.Unlock()

			// Trigger reconnection if we were previously connected
			if previouslyConnected {
				c.logger.Warn("Connection lost, attempting reconnection...")
				go c.attemptReconnect()
			}

			// Notify callback
			c.reconnectingMu.RLock()
			reconnecting := c.reconnecting
			c.reconnectingMu.RUnlock()

			if c.onConnectionStateChange != nil {
				c.onConnectionStateChange(false, reconnecting)
			}

		} else if state == webrtc.PeerConnectionStateClosed {
			c.connected = false
			c.connectedMu.Unlock()

			// Don't reconnect on intentional close
			if c.onConnectionStateChange != nil {
				c.onConnectionStateChange(false, false)
			}
		} else {
			c.connectedMu.Unlock()
		}
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

		// Mark as disconnected and trigger reconnection
		c.connectedMu.Lock()
		wasConnected := c.connected
		c.connected = false
		c.connectedMu.Unlock()

		if wasConnected {
			c.logger.Warn("DataChannel error, attempting reconnection...")
			go c.attemptReconnect()
		}
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

// SendControlStart sends a start command to the server to begin transcription
func (c *Client) SendControlStart() error {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeControlStart,
		Timestamp: time.Now().UnixMilli(),
	}
	return c.SendMessage(msg)
}

// SendControlStop sends a stop command to the server to end transcription
func (c *Client) SendControlStop() error {
	msg := &protocol.Message{
		Type:      protocol.MessageTypeControlStop,
		Timestamp: time.Now().UnixMilli(),
	}
	return c.SendMessage(msg)
}

// SendAudioChunk sends an audio chunk to the server
// If disconnected and reconnecting, chunks are buffered automatically
func (c *Client) SendAudioChunk(data []byte, sampleRate, channels int) error {
	c.sequenceIDMu.Lock()
	seqID := c.sequenceID
	c.sequenceID++
	c.sequenceIDMu.Unlock()

	// Check if we're connected
	c.connectedMu.RLock()
	connected := c.connected
	c.connectedMu.RUnlock()

	c.reconnectingMu.RLock()
	reconnecting := c.reconnecting
	c.reconnectingMu.RUnlock()

	// If disconnected and reconnecting, buffer the chunk
	if !connected && reconnecting {
		c.bufferChunk(data, sampleRate, channels, seqID)
		return nil // Return nil since buffering succeeded
	}

	// If disconnected and NOT reconnecting, return error
	if !connected {
		return fmt.Errorf("not connected and not reconnecting")
	}

	// Connected - send immediately
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

// attemptReconnect handles automatic reconnection with exponential backoff
func (c *Client) attemptReconnect() {
	// Prevent multiple concurrent reconnection attempts
	c.reconnectingMu.Lock()
	if c.reconnecting {
		c.reconnectingMu.Unlock()
		return
	}
	c.reconnecting = true
	c.reconnectingMu.Unlock()

	c.logger.Info("Starting reconnection attempts...")

	// Notify callback
	if c.onConnectionStateChange != nil {
		c.onConnectionStateChange(false, true)
	}

	for {
		c.reconnectingMu.Lock()
		attempts := c.reconnectAttempts
		c.reconnectAttempts++
		c.reconnectingMu.Unlock()

		if attempts >= c.maxReconnectAttempts {
			c.logger.Error("Max reconnection attempts (%d) reached, giving up", c.maxReconnectAttempts)
			c.reconnectingMu.Lock()
			c.reconnecting = false
			c.reconnectingMu.Unlock()

			if c.onConnectionStateChange != nil {
				c.onConnectionStateChange(false, false)
			}
			return
		}

		// Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (max)
		delay := c.reconnectBaseDelay * time.Duration(1<<uint(attempts))
		if delay > 30*time.Second {
			delay = 30 * time.Second
		}

		c.logger.Info("Reconnection attempt %d/%d in %v...", attempts+1, c.maxReconnectAttempts, delay)

		select {
		case <-time.After(delay):
			// Try to reconnect
		case <-c.stopReconnect:
			c.logger.Info("Reconnection stopped by user")
			c.reconnectingMu.Lock()
			c.reconnecting = false
			c.reconnectingMu.Unlock()
			return
		}

		// Close old connection
		if c.pc != nil {
			c.pc.Close()
		}
		if c.wsConn != nil {
			c.wsConn.Close()
		}

		// Attempt to reconnect
		err := c.Connect()
		if err != nil {
			c.logger.Warn("Reconnection attempt %d failed: %v", attempts+1, err)
			continue
		}

		// Wait for DataChannel to open (give it 10 seconds)
		c.logger.Info("Waiting for DataChannel to open...")
		for i := 0; i < 100; i++ {
			time.Sleep(100 * time.Millisecond)
			c.connectedMu.RLock()
			connected := c.connected
			c.connectedMu.RUnlock()

			if connected {
				c.logger.Info("Reconnection successful on attempt %d!", attempts+1)
				return // Success! OnConnectionStateChange will handle the rest
			}
		}

		c.logger.Warn("Reconnection attempt %d timed out waiting for DataChannel", attempts+1)
	}
}

// bufferChunk buffers an audio chunk during reconnection
func (c *Client) bufferChunk(data []byte, sampleRate, channels int, sequenceID uint64) {
	c.chunkBufferMu.Lock()
	defer c.chunkBufferMu.Unlock()

	// Check if buffer is full
	if len(c.chunkBuffer) >= c.maxBufferSize {
		// Drop oldest chunk
		c.chunkBuffer = c.chunkBuffer[1:]
		c.droppedChunks++
		c.logger.Warn("Chunk buffer full, dropped chunk (total dropped: %d)", c.droppedChunks)
	}

	// Make a copy of the data since it might be reused by the caller
	dataCopy := make([]byte, len(data))
	copy(dataCopy, data)

	chunk := bufferedChunk{
		data:       dataCopy,
		sampleRate: sampleRate,
		channels:   channels,
		sequenceID: sequenceID,
		timestamp:  time.Now().UnixMilli(),
	}

	c.chunkBuffer = append(c.chunkBuffer, chunk)
	c.logger.Debug("Buffered chunk seq=%d (buffer size: %d/%d)", sequenceID, len(c.chunkBuffer), c.maxBufferSize)
}

// flushBuffer sends all buffered chunks after reconnection
func (c *Client) flushBuffer() {
	c.chunkBufferMu.Lock()
	chunks := make([]bufferedChunk, len(c.chunkBuffer))
	copy(chunks, c.chunkBuffer)
	c.chunkBuffer = nil // Clear buffer
	dropped := c.droppedChunks
	c.droppedChunks = 0
	c.chunkBufferMu.Unlock()

	if len(chunks) == 0 {
		c.logger.Info("No buffered chunks to flush")
		return
	}

	c.logger.Info("Flushing %d buffered chunks (%d were dropped during disconnect)", len(chunks), dropped)

	for i, chunk := range chunks {
		audioData := protocol.AudioChunkData{
			SampleRate: chunk.sampleRate,
			Channels:   chunk.channels,
			Data:       chunk.data,
			SequenceID: chunk.sequenceID,
		}

		audioJSON, err := json.Marshal(audioData)
		if err != nil {
			c.logger.Error("Failed to marshal buffered chunk %d: %v", i, err)
			continue
		}

		msg := &protocol.Message{
			Type:      protocol.MessageTypeAudioChunk,
			Timestamp: chunk.timestamp,
			Data:      json.RawMessage(audioJSON),
		}

		// Try to send, but don't fail if it doesn't work
		err = c.SendMessage(msg)
		if err != nil {
			c.logger.Warn("Failed to send buffered chunk seq=%d: %v", chunk.sequenceID, err)
			// Continue trying to send the rest
		} else {
			c.logger.Debug("Flushed buffered chunk seq=%d", chunk.sequenceID)
		}

		// Small delay to avoid overwhelming the connection
		time.Sleep(10 * time.Millisecond)
	}

	c.logger.Info("Finished flushing buffered chunks")
}

// Close closes the WebRTC connection
func (c *Client) Close() error {
	c.logger.Info("Closing WebRTC connection")

	// Stop any reconnection attempts
	close(c.stopReconnect)

	c.reconnectingMu.Lock()
	c.reconnecting = false
	c.reconnectingMu.Unlock()

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
