package transcription

import (
	"log"
)

const (
	// RNNoise operates on 10ms frames
	TargetSampleRate   = 16000 // Our audio is 16kHz
	TargetFrameSamples = 160   // 10ms at 16kHz
)

// RNNoiseProcessor handles noise suppression using RNNoise
// Currently a pass-through implementation (no actual denoising)
// TODO: Integrate actual RNNoise when CGO build environment is set up
type RNNoiseProcessor struct {
	buffer []int16 // Buffer for incomplete frames
}

// NewRNNoiseProcessor creates a new RNNoise processor
func NewRNNoiseProcessor(modelPath string) (*RNNoiseProcessor, error) {
	log.Printf("[RNNoise] DISABLED - Using pass-through (no noise suppression)")
	log.Printf("[RNNoise] To enable: Install rnnoise library and rebuild with CGO")

	return &RNNoiseProcessor{
		buffer: make([]int16, 0, TargetFrameSamples),
	}, nil
}

// ProcessChunk processes an audio chunk through RNNoise
// Input: 16-bit PCM samples at 16kHz
// Output: Denoised 16-bit PCM samples at 16kHz
// Currently: Pass-through (no actual denoising)
func (r *RNNoiseProcessor) ProcessChunk(samples []int16) ([]int16, error) {
	// Pass-through implementation - just return the input
	return samples, nil
}

// ProcessBytes processes raw PCM byte data through RNNoise
// Input: Raw PCM bytes (16-bit little-endian) at 16kHz
// Output: Denoised PCM bytes
// Currently: Pass-through (no actual denoising)
func (r *RNNoiseProcessor) ProcessBytes(pcmData []byte) ([]byte, error) {
	// Pass-through implementation - just return the input
	return pcmData, nil
}

// Flush processes any remaining buffered samples
func (r *RNNoiseProcessor) Flush() []int16 {
	// Pass-through - no buffering needed
	return nil
}

// Reset clears the internal buffer
func (r *RNNoiseProcessor) Reset() {
	// Pass-through - nothing to reset
}

// Close releases RNNoise resources
func (r *RNNoiseProcessor) Close() error {
	// Pass-through - no resources to clean up
	return nil
}
