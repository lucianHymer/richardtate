package transcription

import "fmt"

// ParakeetRequest is the JSON message sent to the subprocess
type ParakeetRequest struct {
	Audio      string `json:"audio"`       // Base64 encoded float32 array
	SampleRate int    `json:"sample_rate"` // Always 16000
}

// ParakeetResponse is the JSON message received from the subprocess
type ParakeetResponse struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// ParakeetTranscriber is a lightweight adapter for the shared Parakeet worker
// This mirrors the WhisperAdapter pattern - lightweight per-pipeline adapter for shared resource
type ParakeetTranscriber struct {
	worker *SharedParakeetWorker
}

// NewParakeetTranscriber creates a new Parakeet transcriber adapter using the shared worker
func NewParakeetTranscriber(worker *SharedParakeetWorker) (*ParakeetTranscriber, error) {
	if worker == nil {
		return nil, fmt.Errorf("shared Parakeet worker is required")
	}

	return &ParakeetTranscriber{
		worker: worker,
	}, nil
}

// Transcribe sends audio to the shared worker and returns transcription
// This is thread-safe - the shared worker handles concurrent access
func (pt *ParakeetTranscriber) Transcribe(audioSamples []float32) (string, error) {
	return pt.worker.Transcribe(audioSamples)
}

// Close is a no-op for the adapter (shared worker lives for entire server lifetime)
func (pt *ParakeetTranscriber) Close() error {
	// Don't close the shared worker - it lives for entire server lifetime
	return nil
}
