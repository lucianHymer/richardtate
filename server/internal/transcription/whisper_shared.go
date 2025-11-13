package transcription

import (
	"fmt"
	"sync"

	"github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// SharedWhisperModel wraps a Whisper model for sharing across multiple contexts
type SharedWhisperModel struct {
	model whisper.Model
	mu    sync.RWMutex
	path  string
	log   *logger.ContextLogger
}

// LoadSharedWhisperModel loads a Whisper model once for sharing
func LoadSharedWhisperModel(modelPath string, log *logger.Logger) (*SharedWhisperModel, error) {
	ctxLog := log.With("whisper-model")
	ctxLog.Info("Loading shared Whisper model from %s", modelPath)

	model, err := whisper.New(modelPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load Whisper model: %w", err)
	}

	ctxLog.Info("Shared Whisper model loaded successfully")

	return &SharedWhisperModel{
		model: model,
		path:  modelPath,
		log:   ctxLog,
	}, nil
}

// NewContext creates a new Whisper context from the shared model
func (m *SharedWhisperModel) NewContext() (whisper.Context, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	ctx, err := m.model.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create context from shared model: %w", err)
	}

	return ctx, nil
}

// GetPath returns the model path (for logging/debugging)
func (m *SharedWhisperModel) GetPath() string {
	return m.path
}

// WhisperTranscriberShared handles audio transcription using a shared Whisper model
type WhisperTranscriberShared struct {
	ctx     whisper.Context
	mu      sync.Mutex
	threads uint
	log     *logger.ContextLogger
}

// NewWhisperTranscriberShared creates a transcriber using the shared model
func NewWhisperTranscriberShared(sharedModel *SharedWhisperModel, config WhisperConfig) (*WhisperTranscriberShared, error) {
	log := config.Logger.With("whisper")

	// Create context from shared model
	ctx, err := sharedModel.NewContext()
	if err != nil {
		return nil, fmt.Errorf("failed to create context: %w", err)
	}

	// Configure context
	if config.Language != "" {
		ctx.SetLanguage(config.Language)
	} else {
		ctx.SetLanguage("auto")
	}

	if config.Threads > 0 {
		ctx.SetThreads(config.Threads)
	}

	ctx.SetTranslate(false)
	ctx.SetTokenTimestamps(true)

	// Set initial prompt for technical context
	initialPrompt := "Voice commands for programming. Speaking to computer assistant. Direct address. Imperative mood. Technical instructions. JavaScript, TypeScript, Go, Solidity, Python, React, Node.js. Functions, variables, classes, interfaces, smart contracts, blockchain, API endpoints, database queries. Git commands, terminal operations, code editor."
	ctx.SetInitialPrompt(initialPrompt)

	log.InfoWithFields("Context created from shared model", map[string]interface{}{
		"language": config.Language,
		"threads":  config.Threads,
	})

	return &WhisperTranscriberShared{
		ctx:     ctx,
		threads: config.Threads,
		log:     log,
	}, nil
}

// Transcribe processes audio samples and returns the transcribed text
func (w *WhisperTranscriberShared) Transcribe(audioSamples []float32) (string, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if len(audioSamples) == 0 {
		return "", fmt.Errorf("empty audio samples")
	}

	duration := float64(len(audioSamples)) / 16000.0
	w.log.Debug("Processing %.2fs of audio", duration)

	var fullText string
	segments := []string{}

	err := w.ctx.Process(audioSamples, nil, func(segment whisper.Segment) {
		segments = append(segments, segment.Text)
	}, nil)

	if err != nil {
		return "", fmt.Errorf("failed to process audio: %w", err)
	}

	// Join all segments
	for i, seg := range segments {
		if i > 0 && len(seg) > 0 {
			fullText += " "
		}
		fullText += seg
	}

	return fullText, nil
}

// Close releases the context (but not the shared model)
func (w *WhisperTranscriberShared) Close() error {
	// Context will be garbage collected
	// The shared model stays alive
	return nil
}
