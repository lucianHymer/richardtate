package transcription

import (
	"fmt"
	"log"
	"sync"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
)

// WhisperTranscriber handles audio transcription using Whisper.cpp
type WhisperTranscriber struct {
	model   whisper.Model
	ctx     whisper.Context
	mu      sync.Mutex
	threads uint
}

// WhisperConfig holds configuration for Whisper transcriber
type WhisperConfig struct {
	ModelPath string
	Language  string // "en" or "auto"
	Threads   uint   // Number of threads for processing
}

// NewWhisperTranscriber creates a new Whisper transcriber instance
func NewWhisperTranscriber(config WhisperConfig) (*WhisperTranscriber, error) {
	// Load model
	model, err := whisper.New(config.ModelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load Whisper model: %w", err)
	}

	// Create context
	ctx, err := model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper context: %w", err)
	}

	// Configure context
	if config.Language != "" {
		ctx.SetLanguage(config.Language)
	} else {
		ctx.SetLanguage("auto") // Auto-detect by default
	}

	if config.Threads > 0 {
		ctx.SetThreads(config.Threads)
	}

	// Disable translation (we want transcription only)
	ctx.SetTranslate(false)

	return &WhisperTranscriber{
		model:   model,
		ctx:     ctx,
		mu:      sync.Mutex{},
		threads: config.Threads,
	}, nil
}

// Transcribe processes audio samples and returns the transcribed text
// audioSamples should be mono float32 at 16kHz
func (w *WhisperTranscriber) Transcribe(audioSamples []float32) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(audioSamples) == 0 {
		return "", fmt.Errorf("empty audio samples")
	}

	// Calculate audio statistics for debugging
	var sum, min, max float32
	min = audioSamples[0]
	max = audioSamples[0]
	for _, sample := range audioSamples {
		sum += sample
		if sample < min {
			min = sample
		}
		if sample > max {
			max = sample
		}
	}
	avg := sum / float32(len(audioSamples))

	log.Printf("[Whisper] Audio stats: samples=%d, duration=%.2fs, min=%.4f, max=%.4f, avg=%.4f",
		len(audioSamples), float64(len(audioSamples))/16000.0, min, max, avg)

	// Process audio through Whisper
	// We don't need callbacks for the simple case, use nil
	err := w.ctx.Process(audioSamples, nil, nil, nil)
	if err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	// Collect all segments into a single text
	var fullText string
	segmentCount := 0
	for {
		segment, err := w.ctx.NextSegment()
		if err != nil {
			break // EOF means we've processed all segments
		}
		segmentCount++

		// Append segment text with space
		text := segment.Text
		log.Printf("[Whisper] Segment %d: %q (start=%.2fs, end=%.2fs)",
			segmentCount, text, float64(segment.Start)/100.0, float64(segment.End)/100.0)

		if len(fullText) > 0 && len(text) > 0 {
			fullText += " "
		}
		fullText += text
	}

	log.Printf("[Whisper] Transcription complete: %d segments, text length=%d", segmentCount, len(fullText))
	return fullText, nil
}

// TranscribeWithCallback processes audio and calls the callback for each segment
// This allows streaming results as they become available
func (w *WhisperTranscriber) TranscribeWithCallback(audioSamples []float32, callback func(text string)) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(audioSamples) == 0 {
		return fmt.Errorf("empty audio samples")
	}

	// Create segment callback wrapper
	segmentCallback := func(segment whisper.Segment) {
		if callback != nil {
			callback(segment.Text)
		}
	}

	// Process audio with callback
	err := w.ctx.Process(audioSamples, nil, segmentCallback, nil)
	if err != nil {
		return fmt.Errorf("failed to process audio: %w", err)
	}

	return nil
}

// Close releases resources
func (w *WhisperTranscriber) Close() error {
	// Context and model will be garbage collected
	// No explicit close needed for current Go bindings
	return nil
}

// ConvertPCMToFloat32 converts 16-bit PCM audio to float32 samples
// Expected format: 16kHz mono PCM
func ConvertPCMToFloat32(pcmData []byte) []float32 {
	// PCM is 16-bit (2 bytes per sample)
	numSamples := len(pcmData) / 2
	samples := make([]float32, numSamples)

	for i := 0; i < numSamples; i++ {
		// Read 16-bit little-endian sample
		sample := int16(pcmData[i*2]) | int16(pcmData[i*2+1])<<8

		// Convert to float32 in range [-1.0, 1.0]
		samples[i] = float32(sample) / 32768.0
	}

	return samples
}
