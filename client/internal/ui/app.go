package ui

import (
	"github.com/progrium/darwinkit/macos"
	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/objc"
)

// App manages the macOS application lifecycle
type App struct {
	window        *Window
	calibration   *CalibrationWindow
	onToggle      func() // Called when Ctrl+N pressed
	onCalibration func() // Called when Ctrl+Alt+C pressed
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

// GetWindow returns the preview window
func (a *App) GetWindow() *Window {
	return a.window
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
	objc.DispatchAsync(fn)
}
