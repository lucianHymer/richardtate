package transcription

import (
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// WhisperConfig holds configuration for Whisper transcriber
type WhisperConfig struct {
	ModelPath string
	Language  string // "en" or "auto"
	Threads   uint   // Number of threads for processing
	Logger    *logger.Logger
}
