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
	u.mu.Lock()
	defer u.mu.Unlock()

	if u.recording {
		u.stopRecording()
	} else {
		u.startRecording()
	}
}

func (u *UI) startRecording() {
	window := u.app.GetWindow()

	// Clear previous text
	window.Clear()

	// Show window
	window.Show()

	// Call handler
	if u.onStart != nil {
		if err := u.onStart(); err != nil {
			u.logger.Error("Recording start failed: %v", err)
			window.Hide()
			return
		}
	}

	u.recording = true
}

func (u *UI) stopRecording() {
	window := u.app.GetWindow()

	// Get accumulated text
	text := window.GetText()

	// Call handler
	if u.onStop != nil {
		u.onStop()
	}

	// Paste text if we have any
	if text != "" {
		// Paste in goroutine to not block
		go func() {
			PasteText(text)
		}()
	}

	// Hide window
	window.Hide()

	u.recording = false
}

// AppendTranscription adds a transcription chunk (thread-safe)
func (u *UI) AppendTranscription(text string) {
	// Dispatch to main thread for UI update
	Dispatch(func() {
		u.app.GetWindow().AppendText(text)
	})
}

// IsRecording returns current recording state
func (u *UI) IsRecording() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.recording
}
