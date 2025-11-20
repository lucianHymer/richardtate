package transcription

import (
	"sync"
	"time"

	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// ProcessingStateTracker monitors VAD state and transcription activity
// to determine if the pipeline is actively processing speech.
//
// State transitions:
// - IDLE → PROCESSING: When VAD first detects speech
// - PROCESSING → IDLE: After 1 second of VAD silence + no transcription activity
type ProcessingStateTracker struct {
	mu               sync.RWMutex
	isProcessing     bool
	vadSilent        bool
	lastActivityTime time.Time
	idleTimeout      time.Duration
	stateCallback    func(isProcessing bool)
	stopMonitor      chan struct{}
	log              *logger.ContextLogger
}

// NewProcessingStateTracker creates a new state tracker
func NewProcessingStateTracker(idleTimeout time.Duration, stateCallback func(bool), log *logger.Logger) *ProcessingStateTracker {
	tracker := &ProcessingStateTracker{
		isProcessing:     false,
		vadSilent:        true,
		lastActivityTime: time.Now(),
		idleTimeout:      idleTimeout,
		stateCallback:    stateCallback,
		stopMonitor:      make(chan struct{}),
		log:              log.With("state"),
	}

	// Start monitoring goroutine
	go tracker.monitorState()

	return tracker
}

// NotifySpeechDetected is called when VAD detects speech (non-silence)
func (t *ProcessingStateTracker) NotifySpeechDetected() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.vadSilent = false
	t.lastActivityTime = time.Now()

	// Transition to processing if we weren't already
	if !t.isProcessing {
		t.isProcessing = true
		t.mu.Unlock() // Unlock before callback
		t.log.Info("Processing state: STARTED (speech detected)")
		if t.stateCallback != nil {
			t.stateCallback(true)
		}
		t.mu.Lock() // Re-lock for deferred unlock
	}
}

// NotifySilence is called when VAD detects silence
func (t *ProcessingStateTracker) NotifySilence() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.vadSilent = true
}

// NotifyTranscription is called when a transcription is produced
func (t *ProcessingStateTracker) NotifyTranscription() {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.lastActivityTime = time.Now()
}

// IsProcessing returns current processing state
func (t *ProcessingStateTracker) IsProcessing() bool {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.isProcessing
}

// monitorState runs in background checking for idle timeout
func (t *ProcessingStateTracker) monitorState() {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			t.checkIdleTransition()
		case <-t.stopMonitor:
			return
		}
	}
}

// checkIdleTransition checks if we should transition to idle state
func (t *ProcessingStateTracker) checkIdleTransition() {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Only check if currently processing
	if !t.isProcessing {
		return
	}

	// Conditions for going idle:
	// 1. VAD is currently detecting silence
	// 2. No activity (transcription or speech) for idleTimeout duration
	timeSinceActivity := time.Since(t.lastActivityTime)
	if t.vadSilent && timeSinceActivity >= t.idleTimeout {
		t.isProcessing = false
		t.mu.Unlock() // Unlock before callback
		t.log.Info("Processing state: IDLE (%.1fs of silence, no activity)", timeSinceActivity.Seconds())
		if t.stateCallback != nil {
			t.stateCallback(false)
		}
		t.mu.Lock() // Re-lock for deferred unlock
	}
}

// Stop stops the monitoring goroutine
func (t *ProcessingStateTracker) Stop() {
	close(t.stopMonitor)
}
