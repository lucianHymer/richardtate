package transcription

import (
	"fmt"

	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// ParakeetConfig holds configuration for Parakeet MLX engine
type ParakeetConfig struct {
	ModelPath  string         // Model ID (e.g., "mlx-community/parakeet-tdt-0.6b-v3")
	ScriptPath string         // Path to parakeet_worker.py script
	Logger     *logger.Logger // Logger instance
}

// ASRConfig holds configuration for ASR engine creation
type ASRConfig struct {
	Engine             string              // "whisper" or "parakeet"
	SharedWhisperModel *SharedWhisperModel // For Whisper engine
	WhisperConfig      WhisperConfig       // For Whisper engine
	ParakeetConfig     ParakeetConfig      // For Parakeet engine
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

	case "parakeet":
		return NewParakeetTranscriber(config.ParakeetConfig)

	default:
		return nil, fmt.Errorf("unsupported ASR engine: %s (supported: whisper, parakeet)", engine)
	}
}
