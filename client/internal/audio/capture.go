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
	ctx        *malgo.AllocatedContext
	device     *malgo.Device
	deviceName string // Optional: specify device by name
	isRunning  bool
	mu         sync.Mutex

	// Output channel for audio chunks
	chunks chan AudioChunk

	// Internal state
	sequenceID uint64
	buffer     []byte
	bufferSize int // Target buffer size in bytes
}

// New creates a new audio capturer
// chunkBufferSize determines how many chunks can be queued (recommend 10-20)
// deviceName specifies which device to use (empty = default)
func New(chunkBufferSize int, deviceName string) (*Capturer, error) {
	// Calculate buffer size: 16kHz * 1 channel * 2 bytes/sample * 0.2 seconds
	bytesPerChunk := SampleRate * Channels * (BitsPerSample / 8) * ChunkSizeMS / 1000

	capturer := &Capturer{
		chunks:     make(chan AudioChunk, chunkBufferSize),
		buffer:     make([]byte, 0, bytesPerChunk),
		bufferSize: bytesPerChunk,
		sequenceID: 0,
		deviceName: deviceName,
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

	// List available devices and find the requested one
	fmt.Println("\n=== Available Audio Devices ===")
	infos, err := c.ctx.Devices(malgo.Capture)
	var selectedDeviceID malgo.DeviceID
	foundDevice := false

	if err == nil {
		for i, info := range infos {
			isDefault := info.IsDefault != 0
			fmt.Printf("[%d] %s", i, info.Name())
			if isDefault {
				fmt.Printf(" [DEFAULT]")
			}
			fmt.Println()

			// Check if this is the device we want
			if c.deviceName != "" && info.Name() == c.deviceName {
				selectedDeviceID = info.ID
				foundDevice = true
				fmt.Printf("    ✅ SELECTED (matches config: %s)\n", c.deviceName)
			}
		}
	}
	fmt.Println("================================\n")

	// Configure capture device
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)

	// Use specific device if found
	if foundDevice {
		deviceConfig.Capture.DeviceID = selectedDeviceID.Pointer()
		fmt.Printf("Using specified device: %s\n", c.deviceName)
	} else if c.deviceName != "" {
		fmt.Printf("⚠️  Warning: Device '%s' not found, using default\n", c.deviceName)
	} else {
		fmt.Println("Using default audio device")
	}

	deviceConfig.Capture.Format = Format
	deviceConfig.Capture.Channels = Channels
	deviceConfig.SampleRate = SampleRate
	deviceConfig.Alsa.NoMMap = 1 // Recommended for better compatibility

	// Print the configuration we're using
	fmt.Printf("Capture config: Format=%v, Channels=%d, SampleRate=%d\n\n",
		deviceConfig.Capture.Format, deviceConfig.Capture.Channels, deviceConfig.SampleRate)

	// Data callback - called by malgo when audio data is available
	firstCallback := true
	onRecvFrames := func(pSample2, pSample []byte, framecount uint32) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if !c.isRunning {
			return
		}

		// Debug first few samples to check data sanity
		if firstCallback && len(pSample) >= 20 {
			firstCallback = false
			fmt.Printf("\n📊 First audio data inspection:\n")
			fmt.Printf("   Framecount: %d\n", framecount)
			fmt.Printf("   pSample2 length: %d bytes\n", len(pSample2))
			fmt.Printf("   pSample length: %d bytes\n", len(pSample))

			// Check which buffer has data
			if len(pSample2) > 0 && len(pSample2) >= 20 {
				fmt.Printf("   ⚠️  pSample2 has data! First 20 bytes (hex): ")
				for i := 0; i < 20; i++ {
					fmt.Printf("%02x ", pSample2[i])
				}
				fmt.Printf("\n")
			}

			if len(pSample) >= 20 {
				fmt.Printf("   pSample first 20 bytes (hex): ")
				for i := 0; i < 20; i++ {
					fmt.Printf("%02x ", pSample[i])
				}
				fmt.Printf("\n")

				// Interpret as int16 samples
				fmt.Printf("   First 5 samples as int16: ")
				for i := 0; i < 10 && i < len(pSample); i += 2 {
					sample := int16(pSample[i]) | int16(pSample[i+1])<<8
					fmt.Printf("%d ", sample)
				}
				fmt.Printf("\n\n")
			}
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

	// Check what the device is ACTUALLY configured with
	fmt.Println("\n🔍 Actual Device Configuration:")
	fmt.Printf("   Sample Rate: %d Hz\n", c.device.SampleRate())
	fmt.Printf("   Format: %v\n", c.device.CaptureFormat())
	fmt.Printf("   Channels: %d\n", c.device.CaptureChannels())
	fmt.Println()

	// Warn if sample rate doesn't match
	if c.device.SampleRate() != SampleRate {
		fmt.Printf("⚠️  WARNING: Device is using %d Hz, but we requested %d Hz!\n", c.device.SampleRate(), SampleRate)
		fmt.Printf("⚠️  This will cause audio distortion. The device doesn't support 16kHz.\n\n")
	}

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
