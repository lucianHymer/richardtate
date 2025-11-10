package protocol

import "encoding/json"

// MessageType defines the type of message being sent
type MessageType string

const (
	// Control messages
	MessageTypeControlStart MessageType = "control.start"
	MessageTypeControlStop  MessageType = "control.stop"
	MessageTypeControlPing  MessageType = "control.ping"
	MessageTypeControlPong  MessageType = "control.pong"

	// Audio data
	MessageTypeAudioChunk MessageType = "audio.chunk"

	// Transcription results
	MessageTypeTranscriptPartial MessageType = "transcript.partial"
	MessageTypeTranscriptFinal   MessageType = "transcript.final"

	// Errors
	MessageTypeError MessageType = "error"
)

// Message is the base message structure sent over DataChannel
type Message struct {
	Type      MessageType     `json:"type"`
	Timestamp int64           `json:"timestamp"` // Unix milliseconds
	Data      json.RawMessage `json:"data,omitempty"`
}

// ControlStartData contains transcription settings sent by client
type ControlStartData struct {
	// VAD Settings
	VADEnergyThreshold float64 `json:"vad_energy_threshold"`
	SilenceThresholdMs int     `json:"silence_threshold_ms"`
	MinChunkDurationMs int     `json:"min_chunk_duration_ms"`
	MaxChunkDurationMs int     `json:"max_chunk_duration_ms"`
	SpeechDensityThreshold float64 `json:"speech_density_threshold"`
}

// AudioChunkData contains raw PCM audio data
type AudioChunkData struct {
	SampleRate int    `json:"sample_rate"`
	Channels   int    `json:"channels"`
	Data       []byte `json:"data"` // Base64 encoded PCM data
	SequenceID uint64 `json:"sequence_id"`
}

// TranscriptData contains transcription results
type TranscriptData struct {
	Text       string  `json:"text"`
	IsFinal    bool    `json:"is_final"`
	Confidence float64 `json:"confidence,omitempty"`
	SequenceID uint64  `json:"sequence_id,omitempty"`
}

// ErrorData contains error information
type ErrorData struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// SignalingMessage is used for WebRTC signaling over WebSocket
type SignalingMessage struct {
	Type string          `json:"type"` // "offer", "answer", "ice"
	Data json.RawMessage `json:"data"`
}
