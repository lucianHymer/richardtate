package ui

import (
	"fmt"
	"sort"
	"time"

	"github.com/progrium/darwinkit/helper/action"
	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/macos/foundation"
	"github.com/progrium/darwinkit/objc"
)

// AudioStats holds energy statistics from a recording
type AudioStats struct {
	Min float64
	Max float64
	Avg float64
	P5  float64
	P95 float64
}

// CalibrationWindow manages the calibration wizard
type CalibrationWindow struct {
	nsWindow    appkit.Window
	contentView appkit.View
	step        int  // 1, 2, or 3
	isRecording bool

	// UI elements that get updated
	titleLabel       appkit.TextField
	subtitleLabel    appkit.TextField
	instructionLabel appkit.TextField
	actionButton     appkit.Button
	statsLabel       appkit.TextField
	thresholdLabel   appkit.TextField
	saveButton       appkit.Button
	cancelButton     appkit.Button

	// Stats from recordings
	backgroundStats *AudioStats
	speechStats     *AudioStats
	threshold       float64

	// Callbacks
	onRecord func(duration time.Duration) *AudioStats
	onSave   func(threshold float64) error
	onClose  func()
}

// NewCalibrationWindow creates the calibration wizard
func NewCalibrationWindow() *CalibrationWindow {
	// Window: 500x400, centered
	frame := foundation.Rect{
		Origin: foundation.Point{X: 0, Y: 0},
		Size:   foundation.Size{Width: 500, Height: 400},
	}

	style := appkit.WindowStyleMaskTitled |
		appkit.WindowStyleMaskClosable

	nsWindow := appkit.NewWindowWithContentRectStyleMaskBackingDefer(
		frame,
		style,
		appkit.BackingStoreBuffered,
		false,
	)
	nsWindow.SetTitle("VAD Calibration")
	nsWindow.Center()
	nsWindow.SetLevel(appkit.FloatingWindowLevel)

	// Dark background
	nsWindow.SetBackgroundColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(0.15, 0.15, 0.15, 0.95))

	// Retain to prevent deallocation
	objc.Retain(&nsWindow)

	cw := &CalibrationWindow{
		nsWindow: nsWindow,
		step:     1,
	}

	// Create content view
	contentView := appkit.NewView()
	contentView.SetFrame(frame)
	nsWindow.SetContentView(contentView)
	cw.contentView = contentView

	// Create UI elements
	cw.createUIElements()

	return cw
}

// createUIElements creates all the UI elements for the wizard
func (cw *CalibrationWindow) createUIElements() {
	// Title label
	cw.titleLabel = createLabel(foundation.Rect{
		Origin: foundation.Point{X: 20, Y: 340},
		Size:   foundation.Size{Width: 460, Height: 30},
	})
	cw.titleLabel.SetFont(appkit.Font_BoldSystemFontOfSize(18))
	cw.titleLabel.SetTextColor(appkit.Color_WhiteColor())
	cw.contentView.AddSubview(cw.titleLabel)

	// Subtitle label
	cw.subtitleLabel = createLabel(foundation.Rect{
		Origin: foundation.Point{X: 20, Y: 300},
		Size:   foundation.Size{Width: 460, Height: 30},
	})
	cw.subtitleLabel.SetFont(appkit.Font_BoldSystemFontOfSize(16))
	cw.contentView.AddSubview(cw.subtitleLabel)

	// Instruction label
	cw.instructionLabel = createLabel(foundation.Rect{
		Origin: foundation.Point{X: 20, Y: 220},
		Size:   foundation.Size{Width: 460, Height: 60},
	})
	cw.instructionLabel.SetTextColor(appkit.Color_WhiteColor())
	cw.contentView.AddSubview(cw.instructionLabel)

	// Action button (Start Recording)
	cw.actionButton = appkit.NewButtonWithTitle("Start Recording")
	cw.actionButton.SetFrame(foundation.Rect{
		Origin: foundation.Point{X: 175, Y: 150},
		Size:   foundation.Size{Width: 150, Height: 40},
	})
	cw.actionButton.SetBezelStyle(appkit.BezelStyleRounded)
	action.Set(cw.actionButton, func(sender objc.Object) {
		cw.handleStartRecording()
	})
	cw.contentView.AddSubview(cw.actionButton)

	// Stats label (shows results)
	cw.statsLabel = createLabel(foundation.Rect{
		Origin: foundation.Point{X: 20, Y: 80},
		Size:   foundation.Size{Width: 460, Height: 120},
	})
	cw.statsLabel.SetTextColor(appkit.Color_WhiteColor())
	cw.statsLabel.SetFont(appkit.Font_SystemFontOfSize(12))
	cw.contentView.AddSubview(cw.statsLabel)

	// Threshold label (large, shown in step 3)
	cw.thresholdLabel = createLabel(foundation.Rect{
		Origin: foundation.Point{X: 20, Y: 180},
		Size:   foundation.Size{Width: 460, Height: 40},
	})
	cw.thresholdLabel.SetFont(appkit.Font_BoldSystemFontOfSize(24))
	cw.thresholdLabel.SetTextColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(0.4, 1, 0.6, 1)) // Green
	cw.thresholdLabel.SetHidden(true)
	cw.contentView.AddSubview(cw.thresholdLabel)

	// Save button (step 3)
	cw.saveButton = appkit.NewButtonWithTitle("Save & Close")
	cw.saveButton.SetFrame(foundation.Rect{
		Origin: foundation.Point{X: 280, Y: 30},
		Size:   foundation.Size{Width: 120, Height: 40},
	})
	cw.saveButton.SetBezelStyle(appkit.BezelStyleRounded)
	action.Set(cw.saveButton, func(sender objc.Object) {
		cw.handleSave()
	})
	cw.saveButton.SetHidden(true)
	cw.contentView.AddSubview(cw.saveButton)

	// Cancel button (step 3)
	cw.cancelButton = appkit.NewButtonWithTitle("Cancel")
	cw.cancelButton.SetFrame(foundation.Rect{
		Origin: foundation.Point{X: 100, Y: 30},
		Size:   foundation.Size{Width: 120, Height: 40},
	})
	cw.cancelButton.SetBezelStyle(appkit.BezelStyleRounded)
	action.Set(cw.cancelButton, func(sender objc.Object) {
		cw.handleCancel()
	})
	cw.cancelButton.SetHidden(true)
	cw.contentView.AddSubview(cw.cancelButton)
}

// createLabel creates a text field configured as a label
func createLabel(frame foundation.Rect) appkit.TextField {
	label := appkit.NewTextField()
	label.SetFrame(frame)
	label.SetEditable(false)
	label.SetBordered(false)
	label.SetDrawsBackground(false)
	label.SetSelectable(false)
	return label
}

// SetHandlers sets the callbacks for recording and saving
func (cw *CalibrationWindow) SetHandlers(
	onRecord func(duration time.Duration) *AudioStats,
	onSave func(threshold float64) error,
	onClose func(),
) {
	cw.onRecord = onRecord
	cw.onSave = onSave
	cw.onClose = onClose
}

// Show displays the calibration window and starts at step 1
func (cw *CalibrationWindow) Show() {
	cw.step = 1
	cw.backgroundStats = nil
	cw.speechStats = nil
	cw.isRecording = false

	cw.drawStep1()
	cw.nsWindow.MakeKeyAndOrderFront(nil)
}

// Close hides and cleans up the window
func (cw *CalibrationWindow) Close() {
	cw.nsWindow.OrderOut(nil)
	if cw.onClose != nil {
		cw.onClose()
	}
}

// drawStep1 renders the background recording step
func (cw *CalibrationWindow) drawStep1() {
	cw.titleLabel.SetStringValue("VAD Calibration - Step 1/3")
	cw.subtitleLabel.SetStringValue("Background Noise")
	cw.subtitleLabel.SetTextColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(0.4, 0.8, 1, 1)) // Blue

	cw.instructionLabel.SetStringValue("Stay completely silent.\nThis will record the ambient noise in your environment.")

	cw.actionButton.SetTitle("Start Recording")
	cw.actionButton.SetHidden(false)
	cw.actionButton.SetEnabled(true)

	cw.statsLabel.SetStringValue("")
	cw.thresholdLabel.SetHidden(true)
	cw.saveButton.SetHidden(true)
	cw.cancelButton.SetHidden(true)
}

// drawStep2 renders the speech recording step
func (cw *CalibrationWindow) drawStep2() {
	cw.titleLabel.SetStringValue("VAD Calibration - Step 2/3")
	cw.subtitleLabel.SetStringValue("Speech Recording")
	cw.subtitleLabel.SetTextColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(1, 0.6, 0.4, 1)) // Orange

	cw.instructionLabel.SetStringValue("Speak normally and continuously.\nThis will record your typical speaking volume.")

	cw.actionButton.SetTitle("Start Recording")
	cw.actionButton.SetHidden(false)
	cw.actionButton.SetEnabled(true)

	// Show background stats
	if cw.backgroundStats != nil {
		cw.statsLabel.SetStringValue(fmt.Sprintf("Background: Avg=%.1f, P95=%.1f",
			cw.backgroundStats.Avg, cw.backgroundStats.P95))
	}
}

// drawStep3 renders the results and save/cancel buttons
func (cw *CalibrationWindow) drawStep3() {
	cw.titleLabel.SetStringValue("VAD Calibration - Step 3/3")
	cw.subtitleLabel.SetStringValue("Results")
	cw.subtitleLabel.SetTextColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(0.4, 1, 0.6, 1)) // Green

	cw.instructionLabel.SetStringValue("Calibration complete! Review the results below.")

	cw.actionButton.SetHidden(true)

	// Show all stats
	if cw.backgroundStats != nil && cw.speechStats != nil {
		statsText := fmt.Sprintf("Background: Avg=%.1f, P95=%.1f\nSpeech: Avg=%.1f, P5=%.1f",
			cw.backgroundStats.Avg, cw.backgroundStats.P95,
			cw.speechStats.Avg, cw.speechStats.P5)
		cw.statsLabel.SetStringValue(statsText)
	}

	// Show threshold
	cw.thresholdLabel.SetStringValue(fmt.Sprintf("Recommended Threshold: %.1f", cw.threshold))
	cw.thresholdLabel.SetHidden(false)

	// Show save/cancel buttons
	cw.saveButton.SetHidden(false)
	cw.cancelButton.SetHidden(false)
}

// handleStartRecording initiates a recording for the current step
func (cw *CalibrationWindow) handleStartRecording() {
	if cw.isRecording || cw.onRecord == nil {
		return
	}

	cw.isRecording = true
	duration := 5 * time.Second

	// Update UI to show recording state
	cw.actionButton.SetTitle("Recording... 5s")
	cw.actionButton.SetEnabled(false)

	// Do recording in background
	go func() {
		stats := cw.onRecord(duration)

		// Back to main thread
		Dispatch(func() {
			cw.isRecording = false

			if stats == nil {
				// Recording failed - show error and close
				cw.Close()
				return
			}

			if cw.step == 1 {
				cw.backgroundStats = stats
				cw.step = 2
				cw.drawStep2()
			} else if cw.step == 2 {
				cw.speechStats = stats
				cw.calculateThreshold()
				cw.step = 3
				cw.drawStep3()
			}
		})
	}()
}

// calculateThreshold computes recommended threshold from stats
func (cw *CalibrationWindow) calculateThreshold() {
	// Formula: background_p95 * 1.5
	// This gives buffer above noise floor
	cw.threshold = cw.backgroundStats.P95 * 1.5
}

// handleSave saves the threshold to config
func (cw *CalibrationWindow) handleSave() {
	if cw.onSave != nil {
		err := cw.onSave(cw.threshold)
		if err != nil {
			// Show error - could use NSAlert here
			return
		}
	}
	cw.Close()
}

// handleCancel closes without saving
func (cw *CalibrationWindow) handleCancel() {
	cw.Close()
}

// CalculateAudioStats computes statistics from energy samples
func CalculateAudioStats(energies []float64) *AudioStats {
	if len(energies) == 0 {
		return &AudioStats{}
	}

	// Sort for percentiles
	sorted := make([]float64, len(energies))
	copy(sorted, energies)
	sort.Float64s(sorted)

	// Calculate stats
	var sum float64
	min := sorted[0]
	max := sorted[len(sorted)-1]

	for _, e := range sorted {
		sum += e
	}
	avg := sum / float64(len(sorted))

	// Percentiles
	p5idx := int(float64(len(sorted)-1) * 0.05)
	p95idx := int(float64(len(sorted)-1) * 0.95)

	return &AudioStats{
		Min: min,
		Max: max,
		Avg: avg,
		P5:  sorted[p5idx],
		P95: sorted[p95idx],
	}
}
