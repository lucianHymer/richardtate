package transcription

// ASRTranscriber is the interface for Automatic Speech Recognition engines
// Supports multiple backends: Whisper, Parakeet, etc.
type ASRTranscriber interface {
	// Transcribe processes audio samples and returns transcribed text
	// audioSamples: float32 PCM samples normalized to [-1.0, 1.0]
	// Returns: transcribed text or error
	Transcribe(audioSamples []float32) (string, error)

	// Close releases resources associated with the transcriber
	Close() error
}
