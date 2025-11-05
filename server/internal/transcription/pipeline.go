package transcription

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// TranscriptionPipeline handles the complete audio-to-text pipeline
type TranscriptionPipeline struct {
	whisper     *WhisperTranscriber
	accumulator *AudioAccumulator
	resultChan  chan TranscriptionResult
	mu          sync.RWMutex
	active      bool
}

// TranscriptionResult holds transcription output
type TranscriptionResult struct {
	Text      string
	Timestamp int64 // Unix timestamp in milliseconds
	Error     error
}

// PipelineConfig holds configuration for the transcription pipeline
type PipelineConfig struct {
	WhisperConfig     WhisperConfig
	MinAudioDuration  int // Minimum audio duration in milliseconds
	MaxAudioDuration  int // Maximum audio duration in milliseconds
	ResultChannelSize int // Size of result channel buffer
}

// NewTranscriptionPipeline creates a new transcription pipeline
func NewTranscriptionPipeline(config PipelineConfig) (*TranscriptionPipeline, error) {
	// Create Whisper transcriber
	whisper, err := NewWhisperTranscriber(config.WhisperConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper transcriber: %w", err)
	}

	// Result channel
	resultChanSize := config.ResultChannelSize
	if resultChanSize == 0 {
		resultChanSize = 10
	}
	resultChan := make(chan TranscriptionResult, resultChanSize)

	pipeline := &TranscriptionPipeline{
		whisper:    whisper,
		resultChan: resultChan,
		active:     false,
	}

	// Create accumulator with callback
	pipeline.accumulator = NewAudioAccumulator(AccumulatorConfig{
		MinDuration: durationFromMs(config.MinAudioDuration),
		MaxDuration: durationFromMs(config.MaxAudioDuration),
		SampleRate:  16000,
		ReadyCallback: pipeline.processAudio,
	})

	return pipeline, nil
}

// ProcessChunk processes an incoming audio chunk
func (p *TranscriptionPipeline) ProcessChunk(audioData []byte, timestamp int64) error {
	p.mu.RLock()
	if !p.active {
		p.mu.RUnlock()
		return fmt.Errorf("pipeline not active")
	}
	p.mu.RUnlock()

	// Add chunk to accumulator
	p.accumulator.AddChunk(audioData)

	return nil
}

// processAudio is called when the accumulator has enough audio
func (p *TranscriptionPipeline) processAudio(audioData []byte) {
	// Convert PCM to float32 for Whisper
	samples := ConvertPCMToFloat32(audioData)

	log.Printf("[Pipeline] Processing %.2f seconds of audio (%d samples)",
		float64(len(samples))/16000.0, len(samples))

	// Transcribe
	text, err := p.whisper.Transcribe(samples)

	// Send result
	result := TranscriptionResult{
		Text:      text,
		Timestamp: currentTimeMillis(),
		Error:     err,
	}

	select {
	case p.resultChan <- result:
		if err != nil {
			log.Printf("[Pipeline] Transcription error: %v", err)
		} else {
			log.Printf("[Pipeline] Transcription result: %q", text)
		}
	default:
		log.Printf("[Pipeline] Result channel full, dropping result")
	}
}

// Start activates the pipeline
func (p *TranscriptionPipeline) Start() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if p.active {
		return fmt.Errorf("pipeline already active")
	}

	p.active = true
	p.accumulator.Clear()

	log.Printf("[Pipeline] Started")
	return nil
}

// Stop deactivates the pipeline and flushes remaining audio
func (p *TranscriptionPipeline) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	if !p.active {
		return fmt.Errorf("pipeline not active")
	}

	// Flush any remaining audio
	p.accumulator.Flush()

	p.active = false
	log.Printf("[Pipeline] Stopped")
	return nil
}

// Results returns the channel for receiving transcription results
func (p *TranscriptionPipeline) Results() <-chan TranscriptionResult {
	return p.resultChan
}

// Close releases all resources
func (p *TranscriptionPipeline) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.active = false

	if p.whisper != nil {
		p.whisper.Close()
	}

	close(p.resultChan)

	log.Printf("[Pipeline] Closed")
	return nil
}

// IsActive returns whether the pipeline is currently active
func (p *TranscriptionPipeline) IsActive() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.active
}

// Helper functions

func durationFromMs(ms int) time.Duration {
	if ms == 0 {
		return 0
	}
	return time.Duration(ms) * time.Millisecond
}

func currentTimeMillis() int64 {
	return int64(time.Now().UnixNano() / 1000000)
}
