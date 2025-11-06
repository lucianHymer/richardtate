// +build rnnoise

package transcription

import (
	"context"
	"fmt"
	"unsafe"

	"github.com/xaionaro-go/audio/pkg/audio"
	"github.com/xaionaro-go/audio/pkg/noisesuppression/implementations/rnnoise"
)

const (
	// RNNoise operates on 48kHz audio
	RNNoiseSampleRate = 48000
	// Our pipeline uses 16kHz
	PipelineSampleRate = 16000
	// RNNoise frame size: 10ms at 48kHz = 480 samples
	RNNoiseFrameSize = 480
)

// RNNoiseProcessor handles noise suppression using RNNoise
// Handles sample rate conversion (16kHz <-> 48kHz) automatically
type RNNoiseProcessor struct {
	denoiser     *rnnoise.RNNoise
	buffer16kHz  []int16 // Buffer for incomplete 16kHz input frames
	frameSize16k int     // Equivalent frame size at 16kHz (160 samples)
}

// NewRNNoiseProcessor creates a new RNNoise processor
func NewRNNoiseProcessor(modelPath string) (*RNNoiseProcessor, error) {
	// Create RNNoise denoiser (mono channel)
	// Note: modelPath is currently ignored - rnnoise uses built-in model
	denoiser, err := rnnoise.New(audio.Channel(1))
	if err != nil {
		return nil, fmt.Errorf("failed to create RNNoise denoiser: %w", err)
	}

	fmt.Printf("[RNNoise] Initialized - noise suppression active (16kHz ↔ 48kHz resampling)\n")

	return &RNNoiseProcessor{
		denoiser:     denoiser,
		buffer16kHz:  make([]int16, 0, 160),  // 10ms at 16kHz
		frameSize16k: PipelineSampleRate / 100, // 10ms = 160 samples at 16kHz
	}, nil
}

// ProcessChunk processes an audio chunk through RNNoise
// Input: 16-bit PCM samples at 16kHz
// Output: Denoised 16-bit PCM samples at 16kHz
func (r *RNNoiseProcessor) ProcessChunk(samples []int16) ([]int16, error) {
	if len(samples) == 0 {
		return nil, nil
	}

	inputSamples := len(samples)

	// Add to buffer
	r.buffer16kHz = append(r.buffer16kHz, samples...)

	var output []int16
	framesProcessed := 0

	// Process complete frames
	for len(r.buffer16kHz) >= r.frameSize16k {
		// Extract one frame (160 samples at 16kHz)
		frame16k := r.buffer16kHz[:r.frameSize16k]
		r.buffer16kHz = r.buffer16kHz[r.frameSize16k:]

		// Upsample to 48kHz (160 -> 480 samples)
		frame48k := Upsample16to48(frame16k)

		// Convert to float32 bytes for RNNoise
		input48kBytes := int16ToFloat32Bytes(frame48k)

		// Prepare output buffer (same size)
		output48kBytes := make([]byte, len(input48kBytes))

		// Process through RNNoise (operates at 48kHz)
		ctx := context.Background()
		_, err := r.denoiser.SuppressNoise(ctx, input48kBytes, output48kBytes)
		if err != nil {
			return nil, fmt.Errorf("RNNoise processing failed: %w", err)
		}

		// Convert back to int16
		denoised48k := float32BytesToInt16(output48kBytes)

		// Downsample back to 16kHz (480 -> 160 samples)
		denoised16k := Downsample48to16(denoised48k)

		// Append to output
		output = append(output, denoised16k...)
		framesProcessed++
	}

	if framesProcessed > 0 {
		fmt.Printf("[RNNoise] Processed %d samples → %d frames (16kHz → 48kHz → 16kHz) → %d samples\n",
			inputSamples, framesProcessed, len(output))
	}

	return output, nil
}

// ProcessBytes processes raw PCM byte data through RNNoise
// Input: Raw PCM bytes (16-bit little-endian) at 16kHz
// Output: Denoised PCM bytes at 16kHz
func (r *RNNoiseProcessor) ProcessBytes(pcmData []byte) ([]byte, error) {
	// Convert bytes to int16 samples
	samples := bytesToInt16(pcmData)

	// Process through RNNoise
	denoised, err := r.ProcessChunk(samples)
	if err != nil {
		return nil, err
	}

	// Convert back to bytes
	return int16ToBytes(denoised), nil
}

// Flush processes any remaining buffered samples
func (r *RNNoiseProcessor) Flush() []int16 {
	if len(r.buffer16kHz) == 0 {
		return nil
	}

	// Pad buffer to frame size with zeros
	for len(r.buffer16kHz) < r.frameSize16k {
		r.buffer16kHz = append(r.buffer16kHz, 0)
	}

	// Process the padded frame
	output, _ := r.ProcessChunk(r.buffer16kHz)
	r.buffer16kHz = r.buffer16kHz[:0]

	return output
}

// Reset clears the internal buffers
func (r *RNNoiseProcessor) Reset() {
	r.buffer16kHz = r.buffer16kHz[:0]
}

// Close releases RNNoise resources
func (r *RNNoiseProcessor) Close() error {
	if r.denoiser != nil {
		return r.denoiser.Close()
	}
	return nil
}

// Helper functions for format conversion

func int16ToFloat32Bytes(samples []int16) []byte {
	floats := make([]float32, len(samples))
	for i, sample := range samples {
		// Convert int16 [-32768, 32767] to float32 [-1.0, 1.0]
		floats[i] = float32(sample) / 32768.0
	}

	// Convert float32 slice to bytes (assuming little-endian)
	bytes := make([]byte, len(floats)*4)
	for i, f := range floats {
		bits := *(*uint32)(unsafe.Pointer(&f))
		bytes[i*4] = byte(bits)
		bytes[i*4+1] = byte(bits >> 8)
		bytes[i*4+2] = byte(bits >> 16)
		bytes[i*4+3] = byte(bits >> 24)
	}
	return bytes
}

func float32BytesToInt16(bytes []byte) []int16 {
	numFloats := len(bytes) / 4
	floats := make([]float32, numFloats)

	// Convert bytes to float32 slice (assuming little-endian)
	for i := 0; i < numFloats; i++ {
		bits := uint32(bytes[i*4]) |
			uint32(bytes[i*4+1])<<8 |
			uint32(bytes[i*4+2])<<16 |
			uint32(bytes[i*4+3])<<24
		floats[i] = *(*float32)(unsafe.Pointer(&bits))
	}

	// Convert float32 to int16
	samples := make([]int16, numFloats)
	for i, f := range floats {
		// Convert float32 [-1.0, 1.0] to int16 [-32768, 32767]
		// Clamp to prevent overflow
		if f > 1.0 {
			f = 1.0
		} else if f < -1.0 {
			f = -1.0
		}
		samples[i] = int16(f * 32767.0)
	}
	return samples
}

func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		// Little-endian conversion
		samples[i] = int16(data[i*2]) | int16(data[i*2+1])<<8
	}
	return samples
}

func int16ToBytes(samples []int16) []byte {
	data := make([]byte, len(samples)*2)
	for i, sample := range samples {
		// Little-endian conversion
		data[i*2] = byte(sample)
		data[i*2+1] = byte(sample >> 8)
	}
	return data
}
