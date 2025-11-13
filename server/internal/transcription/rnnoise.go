//go:build !rnnoise
// +build !rnnoise

package transcription

import (
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// This file is used when building WITHOUT the rnnoise build tag
// It provides a pass-through implementation

const (
	// RNNoise operates on 48kHz audio (but pass-through here)
	RNNoiseSampleRate = 48000
	// Our pipeline uses 16kHz
	PipelineSampleRate = 16000
	// RNNoise frame size: 10ms at 48kHz = 480 samples
	RNNoiseFrameSize = 480
)

// RNNoiseProcessor handles noise suppression using RNNoise
// THIS IS THE PASS-THROUGH VERSION (no actual denoising)
type RNNoiseProcessor struct {
	log *logger.ContextLogger
}

// NewRNNoiseProcessor creates a new RNNoise processor (pass-through)
func NewRNNoiseProcessor(modelPath string, log *logger.Logger) (*RNNoiseProcessor, error) {
	contextLog := log.With("rnnoise")
	contextLog.Warn("DISABLED - Using pass-through (build with -tags rnnoise for noise suppression)")

	return &RNNoiseProcessor{
		log: contextLog,
	}, nil
}

// ProcessChunk processes an audio chunk through RNNoise
// Input: 16-bit PCM samples at 16kHz
// Output: Same samples (no processing in pass-through mode)
func (r *RNNoiseProcessor) ProcessChunk(samples []int16) ([]int16, error) {
	// Pass-through - just return the input unchanged
	return samples, nil
}

// ProcessBytes processes raw PCM byte data through RNNoise
// Input: Raw PCM bytes (16-bit little-endian) at 16kHz
// Output: Same bytes (no processing in pass-through mode)
func (r *RNNoiseProcessor) ProcessBytes(pcmData []byte) ([]byte, error) {
	// Pass-through - just return the input unchanged
	return pcmData, nil
}

// Flush processes any remaining buffered samples
func (r *RNNoiseProcessor) Flush() []int16 {
	// Pass-through - no buffering
	return nil
}

// Reset clears the internal buffers
func (r *RNNoiseProcessor) Reset() {
	// Pass-through - nothing to reset
}

// Close releases RNNoise resources
func (r *RNNoiseProcessor) Close() error {
	// Pass-through - no resources to clean up
	return nil
}
