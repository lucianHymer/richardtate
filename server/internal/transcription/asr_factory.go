package transcription

import "fmt"

// ASRConfig holds configuration for ASR engine creation
type ASRConfig struct {
	Engine             string              // "whisper" or "parakeet"
	SharedWhisperModel *SharedWhisperModel // For Whisper engine
	WhisperConfig      WhisperConfig       // For Whisper engine
	// Future: ParakeetConfig will be added here in Phase 2
}

// NewASRTranscriber creates an ASR transcriber based on the engine configuration
func NewASRTranscriber(config ASRConfig) (ASRTranscriber, error) {
	// Default to whisper if engine not specified
	engine := config.Engine
	if engine == "" {
		engine = "whisper"
	}

	switch engine {
	case "whisper":
		return NewWhisperAdapter(config.SharedWhisperModel, config.WhisperConfig)

	// Phase 2: Add parakeet support
	// case "parakeet":
	//     return NewParakeetTranscriber(config.ParakeetConfig)

	default:
		return nil, fmt.Errorf("unsupported ASR engine: %s (supported: whisper)", engine)
	}
}
