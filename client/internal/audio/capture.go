package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
)

const (
	// Audio capture parameters (matching plan requirements)
	SampleRate   = 16000 // 16kHz mono as specified
	Channels     = 1     // Mono
	ChunkSizeMS  = 200   // 200ms chunks (100-200ms range from plan)
	Format       = malgo.FormatS16
	BitsPerSample = 16
)

// AudioChunk represents a captured audio chunk ready for transmission
type AudioChunk struct {
	Data       []byte
	SampleRate int
	Channels   int
	SequenceID uint64
	Timestamp  time.Time
}

// Capturer handles microphone audio capture using malgo
type Capturer struct {
	ctx       *malgo.AllocatedContext
	device    *malgo.Device
	isRunning bool
	mu        sync.Mutex

	// Output channel for audio chunks
	chunks chan AudioChunk

	// Internal state
	sequenceID uint64
	buffer     []byte
	bufferSize int // Target buffer size in bytes
}

// New creates a new audio capturer
// chunkBufferSize determines how many chunks can be queued (recommend 10-20)
func New(chunkBufferSize int) (*Capturer, error) {
	// Calculate buffer size: 16kHz * 1 channel * 2 bytes/sample * 0.2 seconds
	bytesPerChunk := SampleRate * Channels * (BitsPerSample / 8) * ChunkSizeMS / 1000

	capturer := &Capturer{
		chunks:     make(chan AudioChunk, chunkBufferSize),
		buffer:     make([]byte, 0, bytesPerChunk),
		bufferSize: bytesPerChunk,
		sequenceID: 0,
	}

	// Initialize malgo context
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize malgo context: %w", err)
	}
	capturer.ctx = ctx

	return capturer, nil
}

// Start begins audio capture from the default microphone
func (c *Capturer) Start() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.isRunning {
		return fmt.Errorf("capturer already running")
	}

	// Configure capture device
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)
	deviceConfig.Capture.Format = Format
	deviceConfig.Capture.Channels = Channels
	deviceConfig.SampleRate = SampleRate
	deviceConfig.Alsa.NoMMap = 1 // Recommended for better compatibility

	// Data callback - called by malgo when audio data is available
	onRecvFrames := func(pSample2, pSample []byte, framecount uint32) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if !c.isRunning {
			return
		}

		// Append incoming data to buffer
		c.buffer = append(c.buffer, pSample...)

		// If buffer is large enough, emit a chunk
		for len(c.buffer) >= c.bufferSize {
			// Extract chunk
			chunkData := make([]byte, c.bufferSize)
			copy(chunkData, c.buffer[:c.bufferSize])

			// Create chunk
			chunk := AudioChunk{
				Data:       chunkData,
				SampleRate: SampleRate,
				Channels:   Channels,
				SequenceID: c.sequenceID,
				Timestamp:  time.Now(),
			}

			// Send to channel (non-blocking - drop if full)
			select {
			case c.chunks <- chunk:
				c.sequenceID++
			default:
				// Buffer full - log warning (in production, use logger)
				fmt.Printf("[WARN] Audio chunk buffer full, dropping chunk %d\n", c.sequenceID)
			}

			// Remove sent data from buffer
			c.buffer = c.buffer[c.bufferSize:]
		}
	}

	// Initialize capture device
	device, err := malgo.InitDevice(c.ctx.Context, deviceConfig, malgo.DeviceCallbacks{
		Data: onRecvFrames,
	})
	if err != nil {
		return fmt.Errorf("failed to initialize capture device: %w", err)
	}
	c.device = device

	// Start capture
	err = c.device.Start()
	if err != nil {
		c.device.Uninit()
		return fmt.Errorf("failed to start capture device: %w", err)
	}

	c.isRunning = true
	return nil
}

// Stop stops audio capture and releases resources
func (c *Capturer) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.isRunning {
		return nil
	}

	c.isRunning = false

	// Stop and cleanup device
	if c.device != nil {
		c.device.Stop()
		c.device.Uninit()
		c.device = nil
	}

	// Clear buffer
	c.buffer = c.buffer[:0]

	return nil
}

// Chunks returns the channel that receives audio chunks
// This channel should be read continuously to prevent blocking
func (c *Capturer) Chunks() <-chan AudioChunk {
	return c.chunks
}

// Close releases all resources
// Call this when done with the capturer
func (c *Capturer) Close() error {
	if err := c.Stop(); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	// Cleanup context
	if c.ctx != nil {
		_ = c.ctx.Uninit()
		c.ctx.Free()
		c.ctx = nil
	}

	// Close chunks channel
	close(c.chunks)

	return nil
}

// IsRunning returns whether the capturer is currently capturing
func (c *Capturer) IsRunning() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.isRunning
}
