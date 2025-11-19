package transcription

import (
	"fmt"

	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// ParakeetConfig holds configuration for Parakeet MLX engine
type ParakeetConfig struct {
	ModelPath  string         // Model ID (e.g., "mlx-community/parakeet-tdt-0.6b-v3")
	ScriptPath string         // Path to parakeet_worker_streaming.py script
	PythonPath string         // Path to Python interpreter (default: "python3")
	Logger     *logger.Logger // Logger instance
}

// ASRConfig holds configuration for ASR engine creation
type ASRConfig struct {
	Engine                string                 // "whisper" or "parakeet"
	SharedWhisperModel    *SharedWhisperModel    // For Whisper engine (shared across pipelines)
	WhisperConfig         WhisperConfig          // For Whisper engine
	SharedParakeetWorker  *SharedParakeetWorker  // For Parakeet engine (shared across pipelines)
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
		if config.SharedParakeetWorker == nil {
			return nil, fmt.Errorf("SharedParakeetWorker is required for parakeet engine")
		}
		return NewParakeetTranscriber(config.SharedParakeetWorker)

	default:
		return nil, fmt.Errorf("unsupported ASR engine: %s (supported: whisper, parakeet)", engine)
	}
}
