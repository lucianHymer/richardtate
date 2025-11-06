package transcription

// Simple 3x resampler for 16kHz <-> 48kHz conversion
// Since 48000 / 16000 = 3 (perfect integer ratio), we can use simple decimation/interpolation

// Upsample16to48 converts 16kHz audio to 48kHz using linear interpolation
// Input: 16-bit PCM samples at 16kHz
// Output: 16-bit PCM samples at 48kHz (3x length)
func Upsample16to48(input []int16) []int16 {
	if len(input) == 0 {
		return nil
	}

	// Output will be 3x the length
	output := make([]int16, len(input)*3)

	for i := 0; i < len(input); i++ {
		baseIdx := i * 3

		if i < len(input)-1 {
			// Linear interpolation between current and next sample
			curr := input[i]
			next := input[i+1]
			diff := next - curr

			output[baseIdx] = curr
			output[baseIdx+1] = curr + diff/3
			output[baseIdx+2] = curr + 2*diff/3
		} else {
			// Last sample: just repeat
			output[baseIdx] = input[i]
			output[baseIdx+1] = input[i]
			output[baseIdx+2] = input[i]
		}
	}

	return output
}

// Downsample48to16 converts 48kHz audio to 16kHz using simple decimation
// Input: 16-bit PCM samples at 48kHz
// Output: 16-bit PCM samples at 16kHz (1/3 length)
func Downsample48to16(input []int16) []int16 {
	if len(input) == 0 {
		return nil
	}

	// Output will be 1/3 the length
	outputLen := len(input) / 3
	output := make([]int16, outputLen)

	// Take every 3rd sample (with simple averaging for anti-aliasing)
	for i := 0; i < outputLen; i++ {
		idx := i * 3

		// Average the 3 samples to prevent aliasing
		if idx+2 < len(input) {
			sum := int32(input[idx]) + int32(input[idx+1]) + int32(input[idx+2])
			output[i] = int16(sum / 3)
		} else {
			output[i] = input[idx]
		}
	}

	return output
}

// Upsample16to48Float converts 16kHz float32 audio to 48kHz
// Used when RNNoise needs float32 input
func Upsample16to48Float(input []float32) []float32 {
	if len(input) == 0 {
		return nil
	}

	output := make([]float32, len(input)*3)

	for i := 0; i < len(input); i++ {
		baseIdx := i * 3

		if i < len(input)-1 {
			curr := input[i]
			next := input[i+1]
			diff := next - curr

			output[baseIdx] = curr
			output[baseIdx+1] = curr + diff/3
			output[baseIdx+2] = curr + 2*diff/3
		} else {
			output[baseIdx] = input[i]
			output[baseIdx+1] = input[i]
			output[baseIdx+2] = input[i]
		}
	}

	return output
}

// Downsample48to16Float converts 48kHz float32 audio to 16kHz
func Downsample48to16Float(input []float32) []float32 {
	if len(input) == 0 {
		return nil
	}

	outputLen := len(input) / 3
	output := make([]float32, outputLen)

	for i := 0; i < outputLen; i++ {
		idx := i * 3

		if idx+2 < len(input) {
			sum := input[idx] + input[idx+1] + input[idx+2]
			output[i] = sum / 3
		} else {
			output[i] = input[idx]
		}
	}

	return output
}
