package audio

import (
	"fmt"
	"sync"
	"time"

	"github.com/gen2brain/malgo"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

const (
	// Audio capture parameters (matching plan requirements)
	SampleRate    = 16000 // 16kHz mono as specified
	Channels      = 1     // Mono
	ChunkSizeMS   = 200   // 200ms chunks (100-200ms range from plan)
	Format        = malgo.FormatS16
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
	logger     *logger.ContextLogger

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
// log is the logger to use for audio capture logs
func New(chunkBufferSize int, deviceName string, log *logger.Logger) (*Capturer, error) {
	// Calculate buffer size: 16kHz * 1 channel * 2 bytes/sample * 0.2 seconds
	bytesPerChunk := SampleRate * Channels * (BitsPerSample / 8) * ChunkSizeMS / 1000

	capturer := &Capturer{
		chunks:     make(chan AudioChunk, chunkBufferSize),
		buffer:     make([]byte, 0, bytesPerChunk),
		bufferSize: bytesPerChunk,
		sequenceID: 0,
		deviceName: deviceName,
		logger:     log.With("audio"),
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
	c.logger.Info("=== Available Audio Devices ===")
	infos, err := c.ctx.Devices(malgo.Capture)
	var selectedDeviceID malgo.DeviceID
	foundDevice := false

	if err == nil {
		for i, info := range infos {
			isDefault := info.IsDefault != 0
			if isDefault {
				c.logger.Info("[%d] %s [DEFAULT]", i, info.Name())
			} else {
				c.logger.Info("[%d] %s", i, info.Name())
			}

			// Check if this is the device we want
			if c.deviceName != "" && info.Name() == c.deviceName {
				selectedDeviceID = info.ID
				foundDevice = true
				c.logger.Info("    ✅ SELECTED (matches config: %s)", c.deviceName)
			}
		}
	}
	c.logger.Info("================================")

	// Configure capture device
	deviceConfig := malgo.DefaultDeviceConfig(malgo.Capture)

	// Use specific device if found
	if foundDevice {
		deviceConfig.Capture.DeviceID = selectedDeviceID.Pointer()
		c.logger.Info("Using specified device: %s", c.deviceName)
	} else if c.deviceName != "" {
		c.logger.Warn("Device '%s' not found, using default", c.deviceName)
	} else {
		c.logger.Info("Using default audio device")
	}

	deviceConfig.Capture.Format = Format
	deviceConfig.Capture.Channels = Channels
	deviceConfig.SampleRate = SampleRate
	deviceConfig.Alsa.NoMMap = 1 // Recommended for better compatibility

	// Log the configuration we're using
	c.logger.Debug("Capture config: Format=%v, Channels=%d, SampleRate=%d",
		deviceConfig.Capture.Format, deviceConfig.Capture.Channels, deviceConfig.SampleRate)

	// Data callback - called by malgo when audio data is available
	callbackCount := 0
	onRecvFrames := func(pSample2, pSample []byte, framecount uint32) {
		c.mu.Lock()
		defer c.mu.Unlock()

		if !c.isRunning {
			return
		}

		callbackCount++

		// Calculate audio level (RMS) to detect if there's actual sound
		var sum int64
		sampleCount := 0
		for i := 0; i+1 < len(pSample); i += 2 {
			sample := int16(pSample[i]) | int16(pSample[i+1])<<8
			sum += int64(sample) * int64(sample)
			sampleCount++
		}

		var rms float64
		if sampleCount > 0 {
			rms = float64(sum) / float64(sampleCount)
			rms = float64(int(rms*100)) / 100.0 // Round to 2 decimals
		}

		// Log every 10th callback if there's significant audio
		if callbackCount%10 == 0 && rms > 100000 {
			// Find min/max sample values
			var minSample, maxSample int16
			if len(pSample) >= 2 {
				minSample = int16(pSample[0]) | int16(pSample[1])<<8
				maxSample = minSample
				for i := 0; i+1 < len(pSample); i += 2 {
					sample := int16(pSample[i]) | int16(pSample[i+1])<<8
					if sample < minSample {
						minSample = sample
					}
					if sample > maxSample {
						maxSample = sample
					}
				}
			}

			c.logger.DebugWithFields("🎤 Audio level detected", map[string]interface{}{
				"rms_squared": rms,
				"min_sample":  minSample,
				"max_sample":  maxSample,
				"frames":      framecount,
				"bytes":       len(pSample),
			})
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
				// Buffer full - log warning
				c.logger.Warn("Audio chunk buffer full, dropping chunk %d", c.sequenceID)
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
	c.logger.InfoWithFields("🔍 Actual Device Configuration", map[string]interface{}{
		"sample_rate": c.device.SampleRate(),
		"format":      c.device.CaptureFormat(),
		"channels":    c.device.CaptureChannels(),
	})

	// Warn if sample rate doesn't match
	if c.device.SampleRate() != SampleRate {
		c.logger.Warn("Device is using %d Hz, but we requested %d Hz - this will cause audio distortion",
			c.device.SampleRate(), SampleRate)
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
