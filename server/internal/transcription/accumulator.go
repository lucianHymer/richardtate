package transcription

import (
	"sync"
	"time"
)

// AudioAccumulator buffers audio chunks until ready for transcription
type AudioAccumulator struct {
	buffer        []byte
	bufferMu      sync.Mutex
	minDuration   time.Duration // Minimum audio duration before transcribing
	maxDuration   time.Duration // Maximum duration to accumulate
	sampleRate    int           // Audio sample rate (16kHz)
	lastFlush     time.Time
	readyCallback func([]byte) // Called when audio is ready for transcription
}

// AccumulatorConfig holds configuration for the audio accumulator
type AccumulatorConfig struct {
	MinDuration   time.Duration // e.g., 1 second
	MaxDuration   time.Duration // e.g., 3 seconds
	SampleRate    int           // 16000 for 16kHz
	ReadyCallback func([]byte)  // Called when buffer is ready
}

// NewAudioAccumulator creates a new audio accumulator
func NewAudioAccumulator(config AccumulatorConfig) *AudioAccumulator {
	if config.MinDuration == 0 {
		config.MinDuration = 1 * time.Second
	}
	if config.MaxDuration == 0 {
		config.MaxDuration = 3 * time.Second
	}
	if config.SampleRate == 0 {
		config.SampleRate = 16000
	}

	return &AudioAccumulator{
		buffer:        make([]byte, 0, config.SampleRate*2*int(config.MaxDuration.Seconds())), // Pre-allocate for max duration
		minDuration:   config.MinDuration,
		maxDuration:   config.MaxDuration,
		sampleRate:    config.SampleRate,
		lastFlush:     time.Now(),
		readyCallback: config.ReadyCallback,
	}
}

// AddChunk adds an audio chunk to the buffer
// Returns true if the buffer was flushed (transcription triggered)
func (a *AudioAccumulator) AddChunk(chunk []byte) bool {
	a.bufferMu.Lock()
	defer a.bufferMu.Unlock()

	// Append to buffer
	a.buffer = append(a.buffer, chunk...)

	// Calculate current buffer duration
	// 16-bit PCM = 2 bytes per sample
	numSamples := len(a.buffer) / 2
	duration := time.Duration(float64(numSamples)/float64(a.sampleRate)) * time.Second

	// Check if we should flush
	shouldFlush := false

	// Flush if we've exceeded max duration
	if duration >= a.maxDuration {
		shouldFlush = true
	}

	// Flush if we've met min duration and enough time has passed
	if duration >= a.minDuration && time.Since(a.lastFlush) >= a.minDuration {
		shouldFlush = true
	}

	if shouldFlush {
		a.flush()
		return true
	}

	return false
}

// flush sends the accumulated audio for transcription and clears buffer
// Must be called with bufferMu locked
func (a *AudioAccumulator) flush() {
	if len(a.buffer) == 0 {
		return
	}

	// Make a copy of the buffer for the callback
	bufferCopy := make([]byte, len(a.buffer))
	copy(bufferCopy, a.buffer)

	// Clear the buffer
	a.buffer = a.buffer[:0]
	a.lastFlush = time.Now()

	// Call callback asynchronously to avoid blocking
	if a.readyCallback != nil {
		go a.readyCallback(bufferCopy)
	}
}

// Flush forces a flush of the current buffer regardless of duration
func (a *AudioAccumulator) Flush() {
	a.bufferMu.Lock()
	defer a.bufferMu.Unlock()
	a.flush()
}

// BufferDuration returns the current buffer duration
func (a *AudioAccumulator) BufferDuration() time.Duration {
	a.bufferMu.Lock()
	defer a.bufferMu.Unlock()

	numSamples := len(a.buffer) / 2
	return time.Duration(float64(numSamples)/float64(a.sampleRate)) * time.Second
}

// BufferSize returns the current buffer size in bytes
func (a *AudioAccumulator) BufferSize() int {
	a.bufferMu.Lock()
	defer a.bufferMu.Unlock()
	return len(a.buffer)
}

// Clear empties the buffer without triggering callback
func (a *AudioAccumulator) Clear() {
	a.bufferMu.Lock()
	defer a.bufferMu.Unlock()
	a.buffer = a.buffer[:0]
	a.lastFlush = time.Now()
}
