package ui

import (
	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/macos/foundation"
	"github.com/progrium/darwinkit/objc"
)

// Window is a floating transcription preview
type Window struct {
	nsWindow  appkit.Window
	textField appkit.TextField
	text      string // Accumulated transcription
}

// NewWindow creates the floating preview window
func NewWindow() *Window {
	// Window frame: 400x200, will be centered
	frame := foundation.Rect{
		Origin: foundation.Point{X: 0, Y: 0},
		Size:   foundation.Size{Width: 400, Height: 200},
	}

	// Window style: title bar, closable, resizable
	style := appkit.WindowStyleMaskTitled |
		appkit.WindowStyleMaskClosable |
		appkit.WindowStyleMaskResizable

	nsWindow := appkit.NewWindowWithContentRectStyleMaskBackingDefer(
		frame,
		style,
		appkit.BackingStoreBuffered,
		false,
	)
	nsWindow.SetTitle("Dictation Preview")
	nsWindow.Center()

	// CRITICAL: Prevent focus stealing - float above other windows
	nsWindow.SetLevel(appkit.FloatingWindowLevel)

	// CRITICAL: Make window non-interactive (click-through)
	nsWindow.SetIgnoresMouseEvents(true)

	// Retain window to prevent deallocation
	objc.Retain(&nsWindow)

	// Don't release on close - we call Close() manually to destroy old windows
	nsWindow.SetReleasedWhenClosed(false)

	// Set dark background color
	nsWindow.SetBackgroundColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(0.1, 0.1, 0.1, 0.95))

	// Create a wrapping label using TextField_WrappingLabelWithString
	// This automatically configures word wrapping
	textField := appkit.TextField_WrappingLabelWithString("")
	textField.SetFrame(foundation.Rect{
		Origin: foundation.Point{X: 10, Y: 10},
		Size:   foundation.Size{Width: 380, Height: 180},
	})
	textField.SetEditable(false)
	textField.SetSelectable(false)
	textField.SetBordered(false)
	textField.SetDrawsBackground(false)

	// Configure font
	font := appkit.Font_SystemFontOfSize(14)
	textField.SetFont(font)

	// Set sea foam green text color
	textField.SetTextColor(appkit.Color_ColorWithSRGBRedGreenBlueAlpha(0.4, 0.95, 0.7, 1.0))

	nsWindow.ContentView().AddSubview(textField)

	return &Window{
		nsWindow:  nsWindow,
		textField: textField,
	}
}

// Show displays the window without stealing focus
func (w *Window) Show() {
	w.nsWindow.MakeKeyAndOrderFront(nil)
}

// Hide hides the window
func (w *Window) Hide() {
	w.nsWindow.OrderOut(nil)
}

// SetText updates the displayed text
func (w *Window) SetText(text string) {
	w.text = text
	// Limit display to last 500 characters to prevent overflow
	displayText := text
	if len(displayText) > 500 {
		displayText = "..." + displayText[len(displayText)-497:]
	}
	w.textField.SetStringValue(displayText)
}

// AppendText adds text to the display
func (w *Window) AppendText(chunk string) {
	if w.text != "" {
		w.text += " "
	}
	w.text += chunk
	w.SetText(w.text)
}

// GetText returns accumulated text
func (w *Window) GetText() string {
	return w.text
}

// Clear resets the text
func (w *Window) Clear() {
	w.text = ""
	w.textField.SetStringValue("")
}

// Close releases the window resources
func (w *Window) Close() {
	if w.nsWindow.Ptr() != nil {
		w.nsWindow.Close()
	}
}
