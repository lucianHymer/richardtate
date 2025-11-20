package ui

import (
	"time"

	"github.com/progrium/darwinkit/dispatch"
	"github.com/progrium/darwinkit/macos"
	"github.com/progrium/darwinkit/macos/appkit"
)

// App manages the macOS application lifecycle
type App struct {
	window        *Window
	calibration   *CalibrationWindow
	onToggle      func() // Called when Ctrl+N pressed
	onCalibration func() // Called when Ctrl+Alt+C pressed

	// Calibration handlers (stored until windows are created)
	calibrationRecordHandler func(time.Duration) *AudioStats
	calibrationSaveHandler   func(float64) error
}

// NewApp creates a new App instance
func NewApp() *App {
	return &App{}
}

// SetHandlers sets the callbacks for hotkey events
func (a *App) SetHandlers(onToggle, onCalibration func()) {
	a.onToggle = onToggle
	a.onCalibration = onCalibration
}

// SetCalibrationHandlers sets the calibration window handlers
// These are stored and applied when the windows are created in Run()
func (a *App) SetCalibrationHandlers(onRecord func(time.Duration) *AudioStats, onSave func(float64) error) {
	a.calibrationRecordHandler = onRecord
	a.calibrationSaveHandler = onSave
}

// GetWindow returns the preview window
func (a *App) GetWindow() *Window {
	return a.window
}

// RecreateWindow creates a new preview window
// The old window (if any) will be garbage collected automatically
func (a *App) RecreateWindow() {
	a.window = NewWindow()
}

// GetCalibration returns the calibration window
func (a *App) GetCalibration() *CalibrationWindow {
	return a.calibration
}

// Run starts the application (blocks on main thread)
func (a *App) Run() {
	macos.RunApp(func(app appkit.Application, delegate *appkit.ApplicationDelegate) {
		// Set as accessory app (no dock icon)
		app.SetActivationPolicy(appkit.ApplicationActivationPolicyAccessory)

		// Initialize window
		a.window = NewWindow()

		// Initialize calibration window
		a.calibration = NewCalibrationWindow()

		// Apply calibration handlers now that window exists
		if a.calibrationRecordHandler != nil || a.calibrationSaveHandler != nil {
			a.calibration.SetHandlers(
				a.calibrationRecordHandler,
				a.calibrationSaveHandler,
				nil,
			)
		}

		// Register global hotkeys
		RegisterHotkeys(a.onToggle, a.onCalibration)

		// Set up delegate to prevent termination on window close
		delegate.SetApplicationShouldTerminateAfterLastWindowClosed(func(appkit.Application) bool {
			return false
		})
	})
}

// Dispatch runs a function on the main thread (for UI updates from goroutines)
func Dispatch(fn func()) {
	dispatch.MainQueue().DispatchAsync(fn)
}
