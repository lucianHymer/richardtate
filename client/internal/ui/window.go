package ui

import (
	"github.com/progrium/darwinkit/macos/appkit"
	"github.com/progrium/darwinkit/macos/foundation"
	"github.com/progrium/darwinkit/objc"
)

// Window is a floating transcription preview
type Window struct {
	nsWindow   appkit.Window
	textView   appkit.TextView
	scrollView appkit.ScrollView
	text       string // Accumulated transcription
}

// NewWindow creates the floating preview window
func NewWindow() *Window {
	// Window frame: 400x300, will be centered
	frame := foundation.Rect{
		Origin: foundation.Point{X: 0, Y: 0},
		Size:   foundation.Size{Width: 400, Height: 300},
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

	// Create text view
	textView := appkit.NewTextView()
	textView.SetFrame(frame)
	textView.SetEditable(false)
	textView.SetString("Ready to transcribe...")

	// Set text view appearance
	textView.SetBackgroundColor(appkit.Color_WhiteColor())

	// Configure font
	font := appkit.Font_SystemFontOfSize(14)
	textView.SetFont(font)

	// Wrap in scroll view
	scrollView := appkit.NewScrollView()
	scrollView.SetFrame(frame)
	scrollView.SetDocumentView(textView)
	scrollView.SetHasVerticalScroller(true)
	scrollView.SetAutohidesScrollers(true)

	nsWindow.SetContentView(scrollView)

	return &Window{
		nsWindow:   nsWindow,
		textView:   textView,
		scrollView: scrollView,
	}
}

// Show displays the window without stealing focus
func (w *Window) Show() {
	w.nsWindow.OrderFrontRegardless() // Show without activating
}

// Hide hides the window
func (w *Window) Hide() {
	w.nsWindow.OrderOut(nil)
}

// SetText updates the displayed text
func (w *Window) SetText(text string) {
	w.text = text
	w.textView.SetString(text)

	// Scroll to bottom to show latest text
	length := len(text)
	if length > 0 {
		w.textView.ScrollRangeToVisible(foundation.Range{Location: uint(length), Length: 0})
	}
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
	w.textView.SetString("")
}
