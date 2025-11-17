package transcription

import "fmt"

// WhisperAdapter adapts WhisperTranscriberShared to the ASRTranscriber interface
// This is a thin wrapper that allows Whisper to be used interchangeably with other ASR engines
type WhisperAdapter struct {
	transcriber *WhisperTranscriberShared
}

// NewWhisperAdapter creates a new Whisper adapter using the shared model
func NewWhisperAdapter(sharedModel *SharedWhisperModel, config WhisperConfig) (*WhisperAdapter, error) {
	transcriber, err := NewWhisperTranscriberShared(sharedModel, config)
	if err != nil {
		return nil, fmt.Errorf("failed to create Whisper transcriber: %w", err)
	}

	return &WhisperAdapter{
		transcriber: transcriber,
	}, nil
}

// Transcribe processes audio samples and returns transcribed text
func (w *WhisperAdapter) Transcribe(audioSamples []float32) (string, error) {
	return w.transcriber.Transcribe(audioSamples)
}

// Close releases the Whisper context
func (w *WhisperAdapter) Close() error {
	return w.transcriber.Close()
}
