package transcription

import (
	"fmt"
	"log"
	"sync"
	"time"
)

// TranscriptionPipeline handles the complete audio-to-text pipeline
type TranscriptionPipeline struct {
	whisper       *WhisperTranscriber
	accumulator   *AudioAccumulator // DEPRECATED: Will be removed once VAD is added
	audioBuffer   []byte           // Buffer ALL audio for whole-session transcription
	audioBufferMu sync.Mutex
	resultChan    chan TranscriptionResult
	mu            sync.RWMutex
	active        bool
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
// For now, we just accumulate ALL audio and transcribe on Stop()
// TODO: Add VAD for intelligent chunking at natural speech breaks
func (p *TranscriptionPipeline) ProcessChunk(audioData []byte, timestamp int64) error {
	p.mu.RLock()
	if !p.active {
		p.mu.RUnlock()
		return fmt.Errorf("pipeline not active")
	}
	p.mu.RUnlock()

	// Accumulate audio into buffer for whole-session transcription
	p.audioBufferMu.Lock()
	p.audioBuffer = append(p.audioBuffer, audioData...)
	bufferSize := len(p.audioBuffer)
	p.audioBufferMu.Unlock()

	// Log every 5 seconds of audio
	if bufferSize%(16000*2*5) < len(audioData) {
		durationSec := float64(bufferSize) / (16000.0 * 2.0) // 16kHz, 2 bytes per sample
		log.Printf("[Pipeline] Buffered %.1f seconds of audio (%d bytes)", durationSec, bufferSize)
	}

	return nil
}

// processAudio is called when the accumulator has enough audio
func (p *TranscriptionPipeline) processAudio(audioData []byte) {
	log.Printf("[Pipeline] Received %d bytes of PCM audio for transcription", len(audioData))

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

	// Clear audio buffer for new recording session
	p.audioBufferMu.Lock()
	p.audioBuffer = p.audioBuffer[:0]
	p.audioBufferMu.Unlock()

	log.Printf("[Pipeline] Started - buffering audio for whole-session transcription")
	return nil
}

// Stop deactivates the pipeline and transcribes all accumulated audio
func (p *TranscriptionPipeline) Stop() error {
	p.mu.Lock()
	if !p.active {
		p.mu.Unlock()
		return fmt.Errorf("pipeline not active")
	}
	p.active = false
	p.mu.Unlock()

	// Get all accumulated audio
	p.audioBufferMu.Lock()
	audioData := make([]byte, len(p.audioBuffer))
	copy(audioData, p.audioBuffer)
	bufferSize := len(audioData)
	p.audioBufferMu.Unlock()

	log.Printf("[Pipeline] Stopped - transcribing %.1f seconds of audio",
		float64(bufferSize)/(16000.0*2.0))

	// Transcribe the entire recording
	if bufferSize > 0 {
		go p.transcribeSession(audioData)
	} else {
		log.Printf("[Pipeline] No audio to transcribe")
	}

	return nil
}

// transcribeSession transcribes an entire recording session
func (p *TranscriptionPipeline) transcribeSession(audioData []byte) {
	log.Printf("[Pipeline] Transcribing %d bytes of PCM audio", len(audioData))

	// Convert PCM to float32 for Whisper
	samples := ConvertPCMToFloat32(audioData)

	log.Printf("[Pipeline] Converted to %d float32 samples (%.2f seconds)",
		len(samples), float64(len(samples))/16000.0)

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
			log.Printf("[Pipeline] Transcription complete: %q (length=%d)", text, len(text))
		}
	default:
		log.Printf("[Pipeline] Result channel full, dropping result")
	}
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
