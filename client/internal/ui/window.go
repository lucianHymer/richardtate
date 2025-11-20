package ui

import (
	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/macos/foundation"
)

// Window is a floating transcription preview
type Window struct {
	nsWindow     appkit.Window
	textField    appkit.TextField
	text         string // Accumulated transcription
	isProcessing bool   // Whether transcription is currently processing
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

	// Let AppKit and Go GC manage window lifecycle automatically
	// Default behavior: window is released when closed, and Go finalizer
	// cleans up properly when the wrapper is garbage collected

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
	w.updateDisplay()
}

// updateDisplay refreshes the display with current text and processing state
func (w *Window) updateDisplay() {
	// Start with the current text
	displayText := w.text

	// Append "..." if processing
	if w.isProcessing {
		displayText += "..."
	}

	// Limit display to last 500 characters to prevent overflow
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
	w.updateDisplay()
}

// GetText returns accumulated text
func (w *Window) GetText() string {
	return w.text
}

// Clear resets the text
func (w *Window) Clear() {
	w.text = ""
	w.isProcessing = false
	w.textField.SetStringValue("")
}

// SetProcessing updates the processing state and refreshes display
func (w *Window) SetProcessing(processing bool) {
	w.isProcessing = processing
	w.updateDisplay()
}
