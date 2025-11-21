package ui

import (
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"sync"
	"time"

	"github.com/lucianHymer/streaming-transcription/client/internal/audio"
	"github.com/lucianHymer/streaming-transcription/client/internal/config"
	"github.com/lucianHymer/streaming-transcription/client/internal/platform"
	"github.com/lucianHymer/streaming-transcription/shared/logger"
)

// UI provides a high-level interface to the Swift UI subprocess
type UI struct {
	subprocess *Subprocess
	recording  bool
	mu         sync.Mutex
	config     *config.Config
	logger     *logger.ContextLogger
	baseLogger *logger.Logger

	// Callbacks
	onStart func() error
	onStop  func() error

	// For calibration
	calibrationInProgress bool
}

// AudioStats represents audio energy statistics
type AudioStats struct {
	Min   float64
	Max   float64
	Avg   float64
	P5    float64
	P95   float64
	Count int
}

// New creates a new UI instance and starts the Swift subprocess
func New(cfg *config.Config, log *logger.Logger, binaryPath string) (*UI, error) {
	subprocess, err := NewSubprocess(binaryPath)
	if err != nil {
		return nil, err
	}

	return &UI{
		subprocess: subprocess,
		config:     cfg,
		logger:     log.With("ui"),
		baseLogger: log,
	}, nil
}
// SetHandlers sets the start/stop recording handlers
func (u *UI) SetHandlers(onStart, onStop func() error) {
	u.onStart = onStart
	u.onStop = onStop
}

// Run registers hotkeys and keeps the process alive
// This is called for compatibility with the old UI interface
func (u *UI) Run() {
	// Initialize clipboard
	if err := platform.InitClipboard(); err != nil {
		u.logger.Error("Failed to initialize clipboard: %v", err)
	}

	// Register global hotkeys
	platform.RegisterHotkeys(u.toggleRecording, u.handleCalibration)
	u.logger.Info("Hotkeys registered (Ctrl+N = toggle recording, Ctrl+Alt+C = calibration)")

	// Keep process alive
	u.logger.Info("Swift UI subprocess running")
	select {} // Block forever
}

// toggleRecording handles the Ctrl+N hotkey
func (u *UI) toggleRecording() {
	u.mu.Lock()
	isRecording := u.recording
	u.mu.Unlock()

	if isRecording {
		u.StopRecording()
	} else {
		u.StartRecording()
	}
}

// handleCalibration handles the Ctrl+Alt+C hotkey
func (u *UI) handleCalibration() {
	u.logger.Info("Calibration hotkey pressed")
	go u.ShowCalibration() // Run in goroutine to not block hotkey handler
}

// Close terminates the Swift UI subprocess
func (u *UI) Close() error {
	return u.subprocess.Close()
}

// SetTranscription updates the transcription text
func (u *UI) SetTranscription(text string) {
	if err := u.subprocess.SetText(text); err != nil {
		u.logger.Error("Failed to set transcription: %v", err)
	}
}

// SetProcessingState updates the processing indicator
func (u *UI) SetProcessingState(processing bool) {
	if err := u.subprocess.SetProcessing(processing); err != nil {
		u.logger.Error("Failed to set processing state: %v", err)
	}
}

// IsRecording returns current recording state
func (u *UI) IsRecording() bool {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.recording
}

// StartRecording starts a recording session (called by hotkey handler)
func (u *UI) StartRecording() {
	u.mu.Lock()
	u.recording = true
	u.mu.Unlock()

	// Clear previous text
	u.subprocess.ClearText()

	// Call start handler
	if u.onStart != nil {
		if err := u.onStart(); err != nil {
			u.logger.Error("Failed to start recording: %v", err)
			u.mu.Lock()
			u.recording = false
			u.mu.Unlock()
			return
		}
	}

	// Show window
	if err := u.subprocess.Show(); err != nil {
		u.logger.Error("Failed to show subprocess: %v", err)
	}
}

// StopRecording stops the recording session (called by hotkey handler)
// Note: Text accumulation happens in main.go's message handler
// This method is called when the user presses Ctrl+N to stop recording
func (u *UI) StopRecording() {
	// Call stop handler
	if u.onStop != nil {
		if err := u.onStop(); err != nil {
			u.logger.Error("Failed to stop recording: %v", err)
		}
	}

	// Hide window
	if err := u.subprocess.Hide(); err != nil {
		u.logger.Error("Failed to hide subprocess: %v", err)
	}

	u.mu.Lock()
	u.recording = false
	u.mu.Unlock()
}

// GetText would be used to retrieve accumulated text, but in the new architecture,
// text is accumulated in main.go's session state, not in the UI
// This method is here for compatibility but may not be needed

// ShowCalibration shows the calibration wizard
func (u *UI) ShowCalibration() error {
	u.mu.Lock()
	u.calibrationInProgress = true
	u.mu.Unlock()

	// Show calibration window
	if err := u.subprocess.ShowCalibration(); err != nil {
		return err
	}

	// Step 1: Background recording
	if err := u.subprocess.SetCalibrationStep(1); err != nil {
		return err
	}
	if err := u.subprocess.SetCalibrationMessage("Stay silent for 5 seconds..."); err != nil {
		return err
	}

	// Record background
	u.logger.Info("Recording background noise...")
	bgStats := u.recordForCalibration(5 * time.Second)
	if bgStats == nil {
		return fmt.Errorf("failed to record background")
	}

	// Step 2: Speech recording
	if err := u.subprocess.SetCalibrationStep(2); err != nil {
		return err
	}
	if err := u.subprocess.SetCalibrationMessage("Speak normally for 5 seconds..."); err != nil {
		return err
	}

	// Record speech
	u.logger.Info("Recording speech...")
	speechStats := u.recordForCalibration(5 * time.Second)
	if speechStats == nil {
		return fmt.Errorf("failed to record speech")
	}

	// Calculate recommended threshold
	recommended := (bgStats.P95 + speechStats.P5) / 2

	// Step 3: Show results
	if err := u.subprocess.SetCalibrationStep(3); err != nil {
		return err
	}
	if err := u.subprocess.SetCalibrationStats(bgStats.P95, speechStats.P5, recommended); err != nil {
		return err
	}

	// Save threshold
	if err := u.saveCalibrationThreshold(recommended); err != nil {
		u.logger.Error("Failed to save threshold: %v", err)
	}

	// Auto-hide after 3 seconds
	time.Sleep(3 * time.Second)
	u.subprocess.HideCalibration()

	u.mu.Lock()
	u.calibrationInProgress = false
	u.mu.Unlock()

	return nil
}

// recordForCalibration captures audio and returns energy stats
func (u *UI) recordForCalibration(duration time.Duration) *AudioStats {
	capturer, err := audio.New(20, u.config.Audio.DeviceName, u.baseLogger)
	if err != nil {
		u.logger.Error("Failed to create audio capturer: %v", err)
		return nil
	}
	defer capturer.Close()

	var energies []float64
	done := make(chan struct{})

	// Update progress bar
	startTime := time.Now()
	go func() {
		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				elapsed := time.Since(startTime)
				progress := elapsed.Seconds() / duration.Seconds()
				if progress >= 1.0 {
					return
				}
				u.subprocess.SetCalibrationProgress(progress)
			case <-done:
				return
			}
		}
	}()

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

// saveCalibrationThreshold saves the threshold to config
func (u *UI) saveCalibrationThreshold(threshold float64) error {
	// Import the config package's update function
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

// Helper functions

func bytesToInt16(data []byte) []int16 {
	samples := make([]int16, len(data)/2)
	for i := 0; i < len(samples); i++ {
		samples[i] = int16(binary.LittleEndian.Uint16(data[i*2:]))
	}
	return samples
}

func calculateEnergy(samples []int16) float64 {
	var sum float64
	for _, s := range samples {
		sum += float64(s) * float64(s)
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// CalculateAudioStats computes statistical metrics from energy values
func CalculateAudioStats(energies []float64) *AudioStats {
	if len(energies) == 0 {
		return nil
	}

	// Sort for percentile calculation
	sorted := make([]float64, len(energies))
	copy(sorted, energies)
	sort.Float64s(sorted)

	// Calculate statistics
	var sum float64
	min := sorted[0]
	max := sorted[len(sorted)-1]
	for _, e := range energies {
		sum += e
	}
	avg := sum / float64(len(energies))

	// Percentiles
	p5 := percentile(sorted, 0.05)
	p95 := percentile(sorted, 0.95)

	return &AudioStats{
		Min:   min,
		Max:   max,
		Avg:   avg,
		P5:    p5,
		P95:   p95,
		Count: len(energies),
	}
}

func percentile(sorted []float64, p float64) float64 {
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
