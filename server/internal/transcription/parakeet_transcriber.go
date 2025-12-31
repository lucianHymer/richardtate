package transcription

import "fmt"

// ParakeetRequest is the JSON message sent to the subprocess (kept for reference)
type ParakeetRequest struct {
	Audio      string `json:"audio"`       // Base64 encoded float32 array
	SampleRate int    `json:"sample_rate"` // Always 16000
}

// ParakeetResponse is the JSON message received from the subprocess (kept for reference)
type ParakeetResponse struct {
	Text  string `json:"text,omitempty"`
	Error string `json:"error,omitempty"`
}

// ParakeetTranscriber is a lightweight adapter for the shared Parakeet batch worker
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

// Transcribe processes audio through the shared batch worker
// Audio chunks come from the VAD-based chunker (same as Whisper)
func (pt *ParakeetTranscriber) Transcribe(audioSamples []float32) (string, error) {
	return pt.worker.Transcribe(audioSamples)
}

// Close is a no-op since we don't own the shared worker
func (pt *ParakeetTranscriber) Close() error {
	// No-op - shared worker lives for entire server lifetime
	return nil
}
