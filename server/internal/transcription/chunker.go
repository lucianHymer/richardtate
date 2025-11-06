package transcription

import (
	"sync"
	"time"

	"github.com/lucianHymer/streaming-transcription/server/internal/logger"
)

// SmartChunkerConfig holds configuration for VAD-based chunking
type SmartChunkerConfig struct {
	SampleRate         int           // Audio sample rate (16kHz)
	SilenceThreshold   time.Duration // Duration of silence to trigger chunk (1s)
	MinChunkDuration   time.Duration // Minimum chunk duration (avoid tiny chunks)
	MaxChunkDuration   time.Duration // Maximum chunk duration (safety limit)
	VADEnergyThreshold float64       // Energy threshold for VAD
	ChunkReadyCallback func([]int16) // Called when chunk is ready for transcription
	Logger             *logger.Logger
}

// SmartChunker accumulates audio and chunks based on VAD silence detection
type SmartChunker struct {
	config      SmartChunkerConfig
	vad         *VoiceActivityDetector
	buffer      []int16
	bufferMu    sync.Mutex
	startTime   time.Time
	lastChunk   time.Time
	totalSpeech time.Duration
	log         *logger.ContextLogger
}

// NewSmartChunker creates a new VAD-based audio chunker
func NewSmartChunker(config SmartChunkerConfig) *SmartChunker {
	// Set defaults
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}
	if config.SilenceThreshold == 0 {
		config.SilenceThreshold = 1 * time.Second // 1 second of silence
	}
	if config.MinChunkDuration == 0 {
		config.MinChunkDuration = 500 * time.Millisecond // Avoid very short chunks
	}
	if config.MaxChunkDuration == 0 {
		config.MaxChunkDuration = 30 * time.Second // Safety limit
	}
	if config.VADEnergyThreshold == 0 {
		config.VADEnergyThreshold = 100.0
	}

	// Create VAD
	vad := NewVAD(VADConfig{
		SampleRate:         config.SampleRate,
		FrameDurationMs:    10, // 10ms frames (160 samples at 16kHz)
		EnergyThreshold:    config.VADEnergyThreshold,
		SilenceThresholdMs: int(config.SilenceThreshold.Milliseconds()),
	})

	// Create logger
	log := config.Logger.With("chunker")

	return &SmartChunker{
		config:    config,
		vad:       vad,
		buffer:    make([]int16, 0, config.SampleRate*int(config.MaxChunkDuration.Seconds())),
		startTime: time.Now(),
		lastChunk: time.Now(),
		log:       log,
	}
}

// ProcessSamples processes incoming audio samples
// This should be called with denoised samples from RNNoise
func (c *SmartChunker) ProcessSamples(samples []int16) {
	if len(samples) == 0 {
		return
	}

	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	// Add samples to buffer
	c.buffer = append(c.buffer, samples...)

	// Process through VAD in 10ms frames (160 samples at 16kHz)
	frameSize := c.config.SampleRate / 100 // 10ms = 160 samples at 16kHz
	offset := 0

	for offset+frameSize <= len(samples) {
		frame := samples[offset : offset+frameSize]

		// Run VAD on frame
		c.vad.ProcessFrame(frame)

		offset += frameSize
	}

	// Check if we should chunk
	c.checkAndChunk()
}

// checkAndChunk determines if we should trigger a chunk
// Must be called with bufferMu locked
func (c *SmartChunker) checkAndChunk() {
	bufferDuration := c.getBufferDuration()
	shouldChunk := c.vad.ShouldChunk()
	vadStats := c.vad.Stats()

	// Safety: Always chunk if we hit max duration
	if bufferDuration >= c.config.MaxChunkDuration {
		c.flushChunk()
		return
	}

	// Check if VAD detected sufficient silence AND we have enough audio AND enough actual speech
	// This prevents sending chunks that are mostly silence/noise to Whisper (reduces hallucinations)
	minSpeechDuration := 1 * time.Second // Require at least 1 second of actual speech
	if shouldChunk &&
	   bufferDuration >= c.config.MinChunkDuration &&
	   vadStats.SpeechDuration >= minSpeechDuration {
		c.flushChunk()
		return
	}
}

// flushChunk sends accumulated audio for transcription
// Must be called with bufferMu locked
func (c *SmartChunker) flushChunk() {
	if len(c.buffer) == 0 {
		return
	}

	// Make a copy for the callback
	chunk := make([]int16, len(c.buffer))
	copy(chunk, c.buffer)

	vadStats := c.vad.Stats()

	// Clear buffer
	c.buffer = c.buffer[:0]
	c.lastChunk = time.Now()
	c.totalSpeech += vadStats.SpeechDuration

	// Reset VAD state
	c.vad.Reset()

	// Call callback asynchronously
	if c.config.ChunkReadyCallback != nil {
		go c.config.ChunkReadyCallback(chunk)
	}
}

// Flush forces a flush of current buffer (called on Stop)
// Only flushes if there's sufficient speech content to avoid hallucinations
func (c *SmartChunker) Flush() {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	// Check if we have sufficient speech content to transcribe
	vadStats := c.vad.Stats()
	minSpeechDuration := 1 * time.Second // Same threshold as regular chunks
	bufferDuration := c.getBufferDuration()

	if len(c.buffer) == 0 {
		c.log.Debug("Flush called but buffer is empty")
		return
	}

	// Only flush if we have enough actual speech
	// This prevents hallucinations on trailing silence/noise
	if vadStats.SpeechDuration >= minSpeechDuration {
		c.log.Debug("Flushing final chunk with %.2fs of speech", vadStats.SpeechDuration.Seconds())
		c.flushChunk()
	} else {
		c.log.Debug("Discarding final chunk: insufficient speech (%.2fs speech in %.2fs buffer)",
			vadStats.SpeechDuration.Seconds(), bufferDuration.Seconds())
		// Clear buffer without transcribing
		c.buffer = c.buffer[:0]
		c.vad.Reset()
	}
}

// getBufferDuration returns the current buffer duration
// Must be called with bufferMu locked
func (c *SmartChunker) getBufferDuration() time.Duration {
	numSamples := len(c.buffer)
	seconds := float64(numSamples) / float64(c.config.SampleRate)
	return time.Duration(seconds * float64(time.Second))
}

// GetStats returns current chunker statistics
func (c *SmartChunker) GetStats() ChunkerStats {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	return ChunkerStats{
		BufferDuration: c.getBufferDuration(),
		BufferSamples:  len(c.buffer),
		TotalSpeech:    c.totalSpeech,
		TimeSinceChunk: time.Since(c.lastChunk),
		VADStats:       c.vad.Stats(),
	}
}

// Reset clears the chunker state
func (c *SmartChunker) Reset() {
	c.bufferMu.Lock()
	defer c.bufferMu.Unlock()

	c.buffer = c.buffer[:0]
	c.vad.Reset()
	c.startTime = time.Now()
	c.lastChunk = time.Now()
	c.totalSpeech = 0
}

// ChunkerStats holds statistics about the chunker
type ChunkerStats struct {
	BufferDuration time.Duration
	BufferSamples  int
	TotalSpeech    time.Duration
	TimeSinceChunk time.Duration
	VADStats       VADStats
}
