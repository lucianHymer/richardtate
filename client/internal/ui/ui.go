package ui

import (
	"encoding/binary"
	"math"
	"sync"
	"time"

	"github.com/lucianHymer/streaming-transcription/client/internal/audio"
	"github.com/lucianHymer/streaming-transcription/client/internal/config"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// UI manages the native macOS interface
type UI struct {
	app         *App
	recording   bool
	mu          sync.Mutex

	config     *config.Config
	logger     *logger.ContextLogger
	baseLogger *logger.Logger

	// State tracking for graceful finalization
	userWantsToStop  bool // User pressed Ctrl+N to stop
	serverProcessing bool // Server is_processing=true

	// Callbacks
	onStart func() error
	onStop  func() error
}

// New creates a new UI instance
func New(cfg *config.Config, log *logger.Logger) *UI {
	return &UI{
		config:     cfg,
		logger:     log.With("ui"),
		baseLogger: log,
	}
}

// SetHandlers sets the start/stop recording handlers
func (u *UI) SetHandlers(onStart, onStop func() error) {
	u.onStart = onStart
	u.onStop = onStop
}

// Run starts the UI (blocks on main thread)
func (u *UI) Run() {
	// Check accessibility permissions first
	if !EnsureAccessibilityPermissions() {
		u.logger.Info("Waiting for accessibility permissions...")
		WaitForAccessibilityPermissions(func() {
			u.logger.Info("Please grant accessibility permissions in System Preferences")
		})
		u.logger.Info("Accessibility permissions granted!")
	}

	// Initialize clipboard
	if err := InitClipboard(); err != nil {
		panic("Failed to initialize clipboard: " + err.Error())
	}

	// Create app with initialization callback
	u.app = NewApp()

	// Set up hotkey handlers (these are stored on App, not windows)
	u.app.SetHandlers(u.toggleRecording, u.showCalibration)

	// Set calibration handlers to be called after windows are created
	u.app.SetCalibrationHandlers(
		u.recordForCalibration,
		u.saveCalibrationThreshold,
	)

	u.app.Run() // Blocks forever
}

// showCalibration opens the calibration wizard
func (u *UI) showCalibration() {
	u.app.GetCalibration().Show()
}

// recordForCalibration captures audio and returns energy stats
func (u *UI) recordForCalibration(duration time.Duration) *AudioStats {
	capturer, err := audio.New(20, u.config.Audio.DeviceName, u.baseLogger)
	if err != nil {
		u.logger.Error("Failed to create audio capturer: %v", err)
		return nil
	}

	var energies []float64
	done := make(chan struct{})

	go func() {
		for chunk := range capturer.Chunks() {
			// Convert bytes to int16 samples for energy calculation
			samples := bytesToInt16(chunk.Data)
			energy := calculateEnergy(samples)
			energies = append(energies, energy)
		}
		close(done)
	}()

	capturer.Start()
	time.Sleep(duration)
	capturer.Stop()
	<-done

	return CalculateAudioStats(energies)
}

// bytesToInt16 converts a byte slice to int16 samples (little-endian)
func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

// calculateEnergy computes RMS energy of audio samples
func calculateEnergy(samples []int16) float64 {
	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// saveCalibrationThreshold saves the threshold to config
func (u *UI) saveCalibrationThreshold(threshold float64) error {
	err := config.UpdateVADThreshold(u.config.GetFilePath(), threshold)
	if err != nil {
		return err
	}

	// Reload config
	if err := u.config.Reload(); err != nil {
		return err
	}

	u.logger.Info("Calibration saved: threshold = %.1f", threshold)
	return nil
}

// toggleRecording handles hotkey press
func (u *UI) toggleRecording() {
	u.logger.Info("Hotkey pressed")

	// Check current state with lock
	u.mu.Lock()
	isRecording := u.recording
	u.mu.Unlock()

	u.logger.Info("Recording state: %v", isRecording)
	if isRecording {
		u.stopRecording()
	} else {
		u.startRecording()
	}
}

func (u *UI) startRecording() {
	u.logger.Info("startRecording called")

	// Recreate window fresh each time
	u.app.RecreateWindow()
	window := u.app.GetWindow()
	if window == nil {
		u.logger.Error("Window not initialized")
		return
	}
	u.logger.Info("Window recreated")

	// Set recording state
	u.mu.Lock()
	u.recording = true
	u.mu.Unlock()
	u.logger.Info("Recording state set to true")

	// Call handler FIRST (audio start), then show window
	if u.onStart != nil {
		u.logger.Info("Calling onStart handler")
		if err := u.onStart(); err != nil {
			u.logger.Error("Recording start failed: %v", err)
			u.mu.Lock()
			u.recording = false
			u.mu.Unlock()
			return
		}
		u.logger.Info("onStart handler completed successfully")
	}

	// Show window AFTER audio is started
	// Small delay to let runloop process
	time.Sleep(50 * time.Millisecond)
	u.logger.Info("Calling Show()")
	window.Show()
	u.logger.Info("Window.Show() called")
}

func (u *UI) stopRecording() {
	u.logger.Info("stopRecording called")

	// Set user wants to stop flag
	u.mu.Lock()
	u.userWantsToStop = true
	currentlyProcessing := u.serverProcessing
	u.mu.Unlock()

	// Call handler to stop audio and send control.stop
	if u.onStop != nil {
		u.onStop()
	}

	u.logger.Info("Stop recording: userWantsToStop=true, serverProcessing=%v", currentlyProcessing)

	// Check if we can finalize immediately
	if !currentlyProcessing {
		u.logger.Info("Server not processing, finalizing immediately")
		u.finalize()
	} else {
		u.logger.Info("Server still processing, waiting for is_processing=false")

		// Start a timeout goroutine to force finalization after 5 seconds
		go func() {
			time.Sleep(5 * time.Second)
			u.mu.Lock()
			stillWaiting := u.userWantsToStop && u.serverProcessing
			u.mu.Unlock()

			if stillWaiting {
				u.logger.Warn("Timeout waiting for server, forcing finalization")
				// Dispatch to main thread for finalization
				Dispatch(func() {
					u.finalize()
				})
			}
		}()
	}
}

// finalize completes the recording session by pasting text and cleaning up
func (u *UI) finalize() {
	u.logger.Info("finalize called")

	window := u.app.GetWindow()
	if window == nil {
		u.logger.Error("Window not initialized in finalize")
		u.mu.Lock()
		u.recording = false
		u.userWantsToStop = false
		u.mu.Unlock()
		return
	}

	// Get accumulated text
	text := window.GetText()
	u.logger.Info("Finalizing with text: %d chars", len(text))

	// Paste text if we have any
	if text != "" {
		// Paste in goroutine to not block
		go func() {
			PasteText(text)
		}()
	}

	// Hide window (already on main thread)
	window.Hide()

	// Reset state
	u.mu.Lock()
	u.recording = false
	u.userWantsToStop = false
	u.serverProcessing = false
	u.mu.Unlock()

	u.logger.Info("Finalization complete")
}

// SetTranscription replaces the current transcription text (thread-safe)
// Used for streaming where each update contains the full accumulated text
func (u *UI) SetTranscription(text string) {
	// Dispatch to main thread for UI update
	Dispatch(func() {
		window := u.app.GetWindow()
		if window != nil {
			u.logger.Debug("Setting transcription text: %d chars", len(text))
			window.SetText(text)
		} else {
			u.logger.Error("SetTranscription called but window is nil")
		}
	})
}

// AppendTranscription adds a transcription chunk (thread-safe)
// Used for incremental transcription where each chunk is new text
func (u *UI) AppendTranscription(text string) {
	// Dispatch to main thread for UI update
	Dispatch(func() {
		window := u.app.GetWindow()
		if window != nil {
			window.AppendText(text)
		}
	})
}

// IsRecording returns current recording state
func (u *UI) IsRecording() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.recording
}

// SetProcessingState updates the processing state (thread-safe)
func (u *UI) SetProcessingState(processing bool) {
	u.logger.Info("SetProcessingState called: processing=%v", processing)

	// Update state
	u.mu.Lock()
	u.serverProcessing = processing
	wantsToStop := u.userWantsToStop
	u.mu.Unlock()

	// Dispatch to main thread for UI update
	Dispatch(func() {
		window := u.app.GetWindow()
		if window != nil {
			window.SetProcessing(processing)
		}

		// Check if we should finalize (both conditions met)
		if wantsToStop && !processing {
			u.logger.Info("Both conditions met (userWantsToStop && !processing), finalizing")
			u.finalize()
		}
	})
}
