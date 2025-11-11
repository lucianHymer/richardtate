package transcription

import (
	"fmt"
	"sync"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// WhisperTranscriber handles audio transcription using Whisper.cpp
type WhisperTranscriber struct {
	model   whisper.Model
	ctx     whisper.Context
	mu      sync.Mutex
	threads uint
	log     *logger.ContextLogger
}

// WhisperConfig holds configuration for Whisper transcriber
type WhisperConfig struct {
	ModelPath string
	Language  string // "en" or "auto"
	Threads   uint   // Number of threads for processing
	Logger    *logger.Logger
}

// NewWhisperTranscriber creates a new Whisper transcriber instance
func NewWhisperTranscriber(config WhisperConfig) (*WhisperTranscriber, error) {
	// Create logger
	log := config.Logger.With("whisper")

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

	// Set token timestamps for more accurate segment timing
	ctx.SetTokenTimestamps(true)

	// Set initial prompt to improve transcription for technical/programming context
	// This helps Whisper understand we're speaking commands to a computer, not having a conversation
	initialPrompt := "Voice commands for programming. Speaking to computer assistant. Direct address. Imperative mood. Technical instructions. JavaScript, TypeScript, Go, Solidity, Python, React, Node.js. Functions, variables, classes, interfaces, smart contracts, blockchain, API endpoints, database queries. Git commands, terminal operations, code editor."
	ctx.SetInitialPrompt(initialPrompt)

	log.InfoWithFields("Context configured", map[string]interface{}{
		"language":       config.Language,
		"threads":        config.Threads,
		"initial_prompt": initialPrompt[:50] + "...", // Log first 50 chars
	})

	return &WhisperTranscriber{
		model:   model,
		ctx:     ctx,
		mu:      sync.Mutex{},
		threads: config.Threads,
		log:     log,
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

	duration := float64(len(audioSamples)) / 16000.0
	w.log.DebugWithFields("Audio stats", map[string]interface{}{
		"samples":  len(audioSamples),
		"duration": fmt.Sprintf("%.2fs", duration),
		"min":      fmt.Sprintf("%.4f", min),
		"max":      fmt.Sprintf("%.4f", max),
		"avg":      fmt.Sprintf("%.4f", avg),
	})

	// Process audio through Whisper with callback to collect segments
	w.log.Debug("Starting Whisper processing...")

	var fullText string
	segmentCount := 0
	segments := []string{}

	err := w.ctx.Process(audioSamples, nil, func(segment whisper.Segment) {
		segmentCount++
		text := segment.Text
		w.log.DebugWithFields("Segment received", map[string]interface{}{
			"segment": segmentCount,
			"text":    text,
			"start":   fmt.Sprintf("%.2fs", float64(segment.Start)/100.0),
			"end":     fmt.Sprintf("%.2fs", float64(segment.End)/100.0),
		})
		segments = append(segments, text)
	}, nil)

	if err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	// Join all segments
	for i, seg := range segments {
		if i > 0 && len(seg) > 0 {
			fullText += " "
		}
		fullText += seg
	}

	w.log.DebugWithFields("Transcription complete", map[string]interface{}{
		"segments":    segmentCount,
		"text_length": len(fullText),
	})
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
