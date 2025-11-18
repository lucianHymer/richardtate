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

// ParakeetTranscriber is a lightweight adapter for the shared Parakeet streaming worker
// This mirrors the WhisperAdapter pattern - lightweight per-pipeline adapter for shared resource
type ParakeetTranscriber struct {
	worker   *SharedParakeetWorker
	clientID string // Unique client ID for this transcriber's streaming session
}

// NewParakeetTranscriber creates a new Parakeet transcriber adapter using the shared worker
func NewParakeetTranscriber(worker *SharedParakeetWorker) (*ParakeetTranscriber, error) {
	if worker == nil {
		return nil, fmt.Errorf("shared Parakeet worker is required")
	}

	// Create a client for this transcriber's streaming session
	clientID := worker.CreateClient()

	return &ParakeetTranscriber{
		worker:   worker,
		clientID: clientID,
	}, nil
}

// Transcribe processes audio through the streaming interface
// For Parakeet streaming, this buffers internally and returns transcriptions when available
func (pt *ParakeetTranscriber) Transcribe(audioSamples []float32) (string, error) {
	// Process audio through the streaming interface
	// The worker handles buffering and returns text when available
	return pt.worker.ProcessAudio(pt.clientID, audioSamples)
}

// Close cleans up the client session
func (pt *ParakeetTranscriber) Close() error {
	// Close the client session (but not the shared worker)
	return pt.worker.CloseClient(pt.clientID)
}
