package transcription

import (
	"fmt"
	"log"

	"github.com/xaionaro-go/audio/pkg/noisesuppression/implementations/rnnoise"
)

const (
	// RNNoise operates on 10ms frames
	RNNoiseFrameSize    = 480  // 10ms at 48kHz (library default)
	RNNoiseSampleRate   = 48000 // RNNoise expects 48kHz
	TargetSampleRate    = 16000 // Our audio is 16kHz
	TargetFrameSamples  = 160   // 10ms at 16kHz
)

// RNNoiseProcessor handles noise suppression using RNNoise
type RNNoiseProcessor struct {
	denoiser *rnnoise.NoiseSuppressor
	buffer   []int16 // Buffer for incomplete frames
}

// NewRNNoiseProcessor creates a new RNNoise processor
func NewRNNoiseProcessor(modelPath string) (*RNNoiseProcessor, error) {
	// Create RNNoise denoiser
	denoiser, err := rnnoise.New(rnnoise.Options{
		ModelPath: modelPath,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create RNNoise denoiser: %w", err)
	}

	log.Printf("[RNNoise] Initialized with model: %s", modelPath)

	return &RNNoiseProcessor{
		denoiser: denoiser,
		buffer:   make([]int16, 0, TargetFrameSamples),
	}, nil
}

// ProcessChunk processes an audio chunk through RNNoise
// Input: 16-bit PCM samples at 16kHz
// Output: Denoised 16-bit PCM samples at 16kHz
func (r *RNNoiseProcessor) ProcessChunk(samples []int16) ([]int16, error) {
	if len(samples) == 0 {
		return samples, nil
	}

	// Append new samples to buffer
	r.buffer = append(r.buffer, samples...)

	// Process complete frames
	output := make([]int16, 0, len(r.buffer))

	for len(r.buffer) >= TargetFrameSamples {
		// Extract one frame
		frame := r.buffer[:TargetFrameSamples]
		r.buffer = r.buffer[TargetFrameSamples:]

		// Convert int16 to float32 for RNNoise
		floatFrame := make([]float32, len(frame))
		for i, sample := range frame {
			floatFrame[i] = float32(sample) / 32768.0
		}

		// Process through RNNoise
		denoisedFloat, err := r.denoiser.Process(floatFrame)
		if err != nil {
			return nil, fmt.Errorf("RNNoise processing failed: %w", err)
		}

		// Convert back to int16
		denoisedFrame := make([]int16, len(denoisedFloat))
		for i, sample := range denoisedFloat {
			// Clamp and convert
			val := sample * 32768.0
			if val > 32767.0 {
				val = 32767.0
			} else if val < -32768.0 {
				val = -32768.0
			}
			denoisedFrame[i] = int16(val)
		}

		output = append(output, denoisedFrame...)
	}

	return output, nil
}

// ProcessBytes processes raw PCM byte data through RNNoise
// Input: Raw PCM bytes (16-bit little-endian) at 16kHz
// Output: Denoised PCM bytes
func (r *RNNoiseProcessor) ProcessBytes(pcmData []byte) ([]byte, error) {
	if len(pcmData) == 0 {
		return pcmData, nil
	}

	// Convert bytes to int16 samples
	samples := make([]int16, len(pcmData)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(pcmData[i*2]) | int16(pcmData[i*2+1])<<8
	}

	// Process samples
	denoisedSamples, err := r.ProcessChunk(samples)
	if err != nil {
		return nil, err
	}

	// Convert back to bytes
	output := make([]byte, len(denoisedSamples)*2)
	for i, sample := range denoisedSamples {
		output[i*2] = byte(sample)
		output[i*2+1] = byte(sample >> 8)
	}

	return output, nil
}

// Flush processes any remaining buffered samples
func (r *RNNoiseProcessor) Flush() []int16 {
	if len(r.buffer) == 0 {
		return nil
	}

	// If we have incomplete frame, pad with zeros and process
	if len(r.buffer) < TargetFrameSamples {
		padding := make([]int16, TargetFrameSamples-len(r.buffer))
		r.buffer = append(r.buffer, padding...)
	}

	output, err := r.ProcessChunk(nil)
	if err != nil {
		log.Printf("[RNNoise] Error flushing buffer: %v", err)
		return nil
	}

	return output
}

// Reset clears the internal buffer
func (r *RNNoiseProcessor) Reset() {
	r.buffer = r.buffer[:0]
}

// Close releases RNNoise resources
func (r *RNNoiseProcessor) Close() error {
	if r.denoiser != nil {
		// The library may or may not have a Close method
		// Just log that we're cleaning up
		log.Printf("[RNNoise] Processor closed")
	}
	return nil
}
