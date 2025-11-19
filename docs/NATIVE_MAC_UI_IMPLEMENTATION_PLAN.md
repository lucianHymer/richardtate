# Native macOS UI Implementation Plan

**Status**: Implementation Complete - Ready for Mac Testing
**Target**: Replace Hammerspoon dependency with native Go/macOS UI
**Estimated Effort**: 4-6 days

---

## Mac Handoff Notes

### What's Been Done (Linux)
All code has been written and dependencies resolved. Cannot build/test on Linux due to macOS-specific CGO.

**Files Created** (`client/internal/ui/`):
- `app.go` - DarwinKit NSApplication lifecycle
- `window.go` - Floating preview window
- `hotkey.go` - Carbon Events CGO for global hotkeys
- `paste.go` - Clipboard + Cmd+V simulation
- `permissions.go` - Accessibility permission check/prompt
- `calibration.go` - 3-step wizard with button handlers
- `ui.go` - Public interface

**Files Modified**:
- `client/go.mod` - Added DarwinKit v0.5.0 and clipboard v0.7.1
- `client/cmd/client/main.go` - Integrated UI, removed API server
- `go.work` - Updated to go 1.24

**Files Deleted**:
- `hammerspoon/` - Entire directory
- `client/internal/api/` - API server package
- `client/internal/calibrate/` - CLI calibration package

### What Needs Testing on Mac

1. **Build the client**:
   ```bash
   cd /workspace/project/client
   go build -o client ./cmd/client
   ```

2. **Test accessibility permissions flow** - First run should prompt for permissions

3. **Test hotkeys**:
   - Ctrl+N - Toggle recording
   - Ctrl+Alt+C - Open calibration wizard

4. **Test preview window** - Should show without stealing focus

5. **Test paste** - Text should go to clipboard and Cmd+V simulated

6. **Test calibration wizard** - 3-step flow with clickable buttons

### Potential Issues to Watch For

- **DarwinKit API differences** - Code uses new DarwinKit v0.5.0 API (not old macdriver)
- **Import paths** - Using `github.com/progrium/darwinkit/macos/appkit` etc.
- **Button handlers** - Using `action.Set()` from `helper/action` package
- **Carbon hotkey modifiers** - 0x1000=Ctrl, 0x0800=Option, keycodes: n=45, c=8

### After Mac Testing

Once testing is complete and any fixes are made, return to Linux for:
- Final cleanup
- Documentation updates
- PR preparation

---

## Overview

### Goal
Replace the current Hammerspoon-based UI (705 lines of Lua) with a native macOS UI built in Go using `progrium/macdriver`. This eliminates the Hammerspoon dependency, provides faster text insertion via clipboard paste, and includes a native calibration wizard.

### Key Features
1. **Single binary** - No Hammerspoon to install/configure
2. **Faster paste** - Clipboard + Cmd+V vs ~200 chars/sec typing
3. **Native calibration** - Built-in calibration wizard (no CLI needed)
4. **Accessibility check** - Prompts for permissions on first run
5. **No HTTP server** - Direct integration, no localhost API

### Key Decisions (Settled)
- **API server**: Remove entirely (not just WebSocket) - no `client/internal/api/`
- **CLI calibration**: Remove entirely - replaced by native UI (no `client/internal/calibrate/`)
- **Calibration hotkey**: Ctrl+Alt+C (same as Hammerspoon)
- **Preview window**: Non-interactive during dictation (`SetIgnoresMouseEvents(true)`)
- **Calibration window**: Interactive (buttons clickable)
- **Accessibility flow**: Check on startup, prompt and wait if not granted
- **Energy calculation**: Done client-side (not sent to server for analysis)

### Current Architecture (What We're Replacing)
```
User presses Ctrl+N
  → Hammerspoon catches hotkey
  → HTTP POST /start to client daemon (localhost:8081)
  → Client starts audio capture
  → Audio → WebRTC → Server → Whisper/Parakeet
  → Transcription → WebRTC → Client
  → Client broadcasts to WebSocket
  → Hammerspoon receives on WebSocket
  → hs.eventtap.keyStrokes() types char-by-char (SLOW!)
```

### New Architecture (What We're Building)
```
User presses Ctrl+N
  → Go client catches hotkey (macdriver Carbon Events)
  → Start audio capture directly (no HTTP call)
  → Audio → WebRTC → Server → Whisper/Parakeet
  → Transcription → WebRTC → Client
  → Client updates floating NSWindow (no focus stealing)
  → On stop: clipboard copy + Cmd+V paste (INSTANT!)

User presses Ctrl+Alt+C
  → Go client catches hotkey
  → Opens calibration wizard window
  → Records background/speech audio
  → Calculates threshold
  → Saves to config
```

### Benefits
1. **Single binary** - No Hammerspoon to install/configure
2. **Faster paste** - Clipboard + Cmd+V vs ~200 chars/sec typing
3. **Native windows** - Real NSWindow, not Hammerspoon canvas
4. **Built-in calibration** - No separate CLI wizard needed
5. **Better UX** - No focus stealing, instant paste
6. **Permission handling** - Automatic accessibility permission prompts
7. **Simpler architecture** - No HTTP API server, direct integration

---

## Dependencies

### Go Packages to Add

```go
// go.mod additions
require (
    github.com/progrium/macdriver v0.5.0  // macOS Cocoa bindings
    golang.design/x/clipboard v0.7.0       // Cross-platform clipboard
)
```

### System Requirements
- **macOS only** - macdriver uses Cocoa/Carbon APIs
- **Accessibility permissions** - Required for Cmd+V simulation
- **CGO enabled** - macdriver requires CGO

---

## File Structure

Create new package: `client/internal/ui/`

```
client/internal/ui/
├── app.go           # NSApplication lifecycle, main thread coordination
├── window.go        # Floating preview window (NSWindow + NSTextView)
├── calibration.go   # 3-step calibration wizard window
├── hotkey.go        # Global hotkey registration (Carbon Event Manager)
├── paste.go         # Clipboard write + Cmd+V simulation
├── permissions.go   # Accessibility permission check/prompt
└── ui.go            # Public interface tying it all together
```

---

## Implementation Details

### 1. app.go - Application Lifecycle

**Purpose**: Initialize Cocoa app and manage main thread.

**Key Concept**: macOS UI MUST run on main thread. All UI updates from other goroutines must be dispatched.

```go
package ui

import (
    "github.com/progrium/macdriver/cocoa"
    "github.com/progrium/macdriver/core"
    "github.com/progrium/macdriver/objc"
)

// App manages the macOS application lifecycle
type App struct {
    window        *Window
    hotkey        *Hotkey
    onToggle      func() // Called when Ctrl+N pressed
    onCalibration func() // Called when Ctrl+Alt+C pressed
}

// Run starts the application (blocks on main thread)
func (a *App) Run() {
    cocoa.TerminateAfterWindowsClose = false

    app := cocoa.NSApp_WithDidLaunch(func(n objc.Object) {
        // Initialize window
        a.window = NewWindow()

        // Register hotkeys
        RegisterHotkeys(a.onToggle, a.onCalibration)

        // Hide dock icon (optional - menubar app style)
        cocoa.NSApp().SetActivationPolicy(cocoa.NSApplicationActivationPolicyAccessory)
    })

    app.Run() // Blocks forever
}

// Dispatch runs a function on the main thread (for UI updates)
func Dispatch(fn func()) {
    core.Dispatch(fn)
}
```

**Important**: The `Run()` function blocks forever. You'll need to call it from `main()` after setting up everything else.

---

### 2. window.go - Floating Preview Window

**Purpose**: Display transcription text without stealing focus during dictation.

**Design Decisions**:
- Window visibility serves as the recording indicator (no separate indicator needed)
- Window hides automatically after paste for clean UX
- Window floats above all other windows but doesn't steal focus
- **Window is non-interactive during dictation** - mouse events pass through to app below

**Key APIs**:
- `SetLevel(NSFloatingWindowLevel)` - Window stays on top
- `OrderFrontRegardless()` - Show without activating/focusing
- `SetIgnoresMouseEvents(true)` - Make window non-interactive (click-through)

**Note**: This is different from the calibration window which IS interactive (has buttons).

```go
package ui

import (
    "github.com/progrium/macdriver/cocoa"
    "github.com/progrium/macdriver/core"
)

// Window is a floating transcription preview
type Window struct {
    nsWindow cocoa.NSWindow
    textView cocoa.NSTextView
    text     string // Accumulated transcription
}

// NewWindow creates the floating preview window
func NewWindow() *Window {
    // Window frame: 400x300, will be centered
    frame := core.Rect(0, 0, 400, 300)

    // Window style: title bar, closable, resizable
    style := cocoa.NSWindowStyleMaskTitled |
             cocoa.NSWindowStyleMaskClosable |
             cocoa.NSWindowStyleMaskResizable

    nsWindow := cocoa.NSWindow_Init(frame, style, cocoa.NSBackingStoreBuffered, false)
    nsWindow.SetTitle("Dictation Preview")
    nsWindow.Center()

    // CRITICAL: Prevent focus stealing
    nsWindow.SetLevel(cocoa.NSFloatingWindowLevel)

    // CRITICAL: Make window non-interactive (click-through)
    // This is for the preview window during dictation
    nsWindow.SetIgnoresMouseEvents(true)

    // Create text view
    textView := cocoa.NSTextView_Init(frame)
    textView.SetEditable(false)
    textView.SetString("Ready to transcribe...")

    // Wrap in scroll view
    scrollView := cocoa.NSScrollView_Init(frame)
    scrollView.SetDocumentView(textView)
    scrollView.SetHasVerticalScroller(true)

    nsWindow.SetContentView(scrollView)

    return &Window{
        nsWindow: nsWindow,
        textView: textView,
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
    w.textView.ScrollRangeToVisible(core.Range(len(text), 0))
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
```

---

### 3. hotkey.go - Global Hotkey Registration

**Purpose**: Register system-wide hotkey (Ctrl+N) that works even when app isn't focused.

**Key Concept**: Uses Carbon Event Manager (legacy but still works) via macdriver.

```go
package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Carbon -framework Cocoa
#include <Carbon/Carbon.h>

extern void hotkeyCallback();

OSStatus hotkeyHandler(EventHandlerCallRef nextHandler, EventRef event, void *userData) {
    hotkeyCallback();
    return noErr;
}

void registerHotkey(int keyCode, int modifiers) {
    EventHotKeyID hotKeyID = {'htky', 1};
    EventHotKeyRef hotKeyRef;
    EventTypeSpec eventType = {kEventClassKeyboard, kEventHotKeyPressed};

    InstallApplicationEventHandler(&hotkeyHandler, 1, &eventType, NULL, NULL);
    RegisterEventHotKey(keyCode, modifiers, hotKeyID, GetApplicationEventTarget(), 0, &hotKeyRef);
}
*/
import "C"

import "sync"

var (
    hotkeyCallback func()
    hotkeyMu       sync.Mutex
)

//export hotkeyCallback
func hotkeyCallback() {
    hotkeyMu.Lock()
    cb := hotkeyCallback
    hotkeyMu.Unlock()

    if cb != nil {
        cb()
    }
}

var (
    toggleCallback      func()
    calibrationCallback func()
    hotkeyMu            sync.Mutex
)

//export hotkeyHandler
func hotkeyHandler(id C.int) {
    hotkeyMu.Lock()
    var cb func()
    if id == 1 {
        cb = toggleCallback
    } else if id == 2 {
        cb = calibrationCallback
    }
    hotkeyMu.Unlock()

    if cb != nil {
        cb()
    }
}

// RegisterHotkeys registers both global hotkeys
// Ctrl+N (toggle recording) and Ctrl+Alt+C (calibration)
func RegisterHotkeys(onToggle, onCalibration func()) {
    hotkeyMu.Lock()
    toggleCallback = onToggle
    calibrationCallback = onCalibration
    hotkeyMu.Unlock()

    // Key codes: https://gist.github.com/eegrok/949034
    // 'n' = 45, 'c' = 8

    // Modifiers:
    // control = 0x1000 (controlKey)
    // option  = 0x0800 (optionKey)
    // command = 0x0100 (cmdKey)
    // shift   = 0x0200 (shiftKey)

    // Hotkey 1: Ctrl+N (toggle recording)
    C.registerHotkey(45, 0x1000, 1)

    // Hotkey 2: Ctrl+Alt+C (calibration)
    // Control + Option = 0x1000 | 0x0800 = 0x1800
    C.registerHotkey(8, 0x1800, 2)
}

// Common key codes for reference:
// 'n' = 45, 'c' = 8, 'v' = 9, 'space' = 49
// Modifiers: control = 0x1000, option = 0x0800, command = 0x0100, shift = 0x0200
```

**Note**: The CGO code is necessary for Carbon Event Manager. This is the standard way to register global hotkeys on macOS.

---

### 4. paste.go - Clipboard and Paste Simulation

**Purpose**: Copy text to clipboard and simulate Cmd+V to paste into active app.

```go
package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices
#include <ApplicationServices/ApplicationServices.h>

void simulatePaste() {
    // Create key down event for Cmd+V
    CGEventRef keyDown = CGEventCreateKeyboardEvent(NULL, 9, true);  // 9 = 'v'
    CGEventSetFlags(keyDown, kCGEventFlagMaskCommand);

    // Create key up event
    CGEventRef keyUp = CGEventCreateKeyboardEvent(NULL, 9, false);
    CGEventSetFlags(keyUp, kCGEventFlagMaskCommand);

    // Post events
    CGEventPost(kCGHIDEventTap, keyDown);
    CGEventPost(kCGHIDEventTap, keyUp);

    // Release
    CFRelease(keyDown);
    CFRelease(keyUp);
}
*/
import "C"

import (
    "golang.design/x/clipboard"
)

// InitClipboard initializes the clipboard (call once at startup)
func InitClipboard() error {
    return clipboard.Init()
}

// PasteText copies text to clipboard and simulates Cmd+V
func PasteText(text string) {
    // Write to clipboard
    clipboard.Write(clipboard.FmtText, []byte(text))

    // Small delay to ensure clipboard is ready
    // (usually not needed but safe)

    // Simulate Cmd+V
    C.simulatePaste()
}
```

**Important**: This requires **Accessibility permissions** in System Preferences → Security & Privacy → Privacy → Accessibility. The app will need to be added to this list.

---

### 5. permissions.go - Accessibility Permission Check

**Purpose**: Check for accessibility permissions on startup and prompt user to grant them if missing.

**Key Concept**: macOS requires explicit user permission for apps that simulate keyboard events (Cmd+V paste). Without this, the paste will silently fail.

```go
package ui

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework ApplicationServices -framework AppKit
#include <ApplicationServices/ApplicationServices.h>
#include <AppKit/AppKit.h>

// Check if we have accessibility permissions
int checkAccessibilityPermissions() {
    return AXIsProcessTrusted();
}

// Prompt user for accessibility permissions
// Opens System Preferences to the right pane
void promptForAccessibility() {
    NSDictionary *options = @{(__bridge NSString *)kAXTrustedCheckOptionPrompt: @YES};
    AXIsProcessTrustedWithOptions((__bridge CFDictionaryRef)options);
}

// Open System Preferences directly to Accessibility pane
void openAccessibilityPreferences() {
    NSString *urlString = @"x-apple.systempreferences:com.apple.preference.security?Privacy_Accessibility";
    NSURL *url = [NSURL URLWithString:urlString];
    [[NSWorkspace sharedWorkspace] openURL:url];
}
*/
import "C"

import (
    "fmt"
    "time"
)

// HasAccessibilityPermissions checks if the app has accessibility permissions
func HasAccessibilityPermissions() bool {
    return C.checkAccessibilityPermissions() != 0
}

// RequestAccessibilityPermissions prompts the user for accessibility permissions
// This shows the system dialog and opens System Preferences
func RequestAccessibilityPermissions() {
    C.promptForAccessibility()
}

// OpenAccessibilityPreferences opens System Preferences to the Accessibility pane
func OpenAccessibilityPreferences() {
    C.openAccessibilityPreferences()
}

// EnsureAccessibilityPermissions checks permissions and prompts if needed
// Returns true if permissions are granted, false if user needs to grant them
func EnsureAccessibilityPermissions() bool {
    if HasAccessibilityPermissions() {
        return true
    }

    // Prompt user - this shows system dialog
    RequestAccessibilityPermissions()

    // Give user time to respond (the dialog is async)
    // We'll check again after a short delay
    time.Sleep(500 * time.Millisecond)

    return HasAccessibilityPermissions()
}

// WaitForAccessibilityPermissions blocks until permissions are granted
// Shows a message and polls periodically
func WaitForAccessibilityPermissions(showAlert func(title, message string)) {
    if HasAccessibilityPermissions() {
        return
    }

    // Show initial prompt
    RequestAccessibilityPermissions()

    // Show alert explaining what to do
    if showAlert != nil {
        showAlert(
            "Accessibility Permission Required",
            "This app needs accessibility permissions to paste text.\n\n"+
                "Please enable it in System Preferences → Security & Privacy → "+
                "Privacy → Accessibility.\n\n"+
                "The app will start automatically once permission is granted.",
        )
    }

    // Poll until permissions are granted
    for !HasAccessibilityPermissions() {
        time.Sleep(1 * time.Second)
    }
}
```

**Startup Flow**:
1. App launches
2. Check `HasAccessibilityPermissions()`
3. If false, call `RequestAccessibilityPermissions()` which:
   - Shows system dialog asking for permission
   - Opens System Preferences to Accessibility pane
4. Wait/poll until user grants permission
5. Continue with normal startup

**User Experience**:
- First run: User sees system dialog + System Preferences opens
- User enables the app in the list
- App automatically continues once enabled
- Subsequent runs: No prompt (already authorized)

---

### 6. calibration.go - VAD Calibration Wizard

**Purpose**: Native 3-step wizard for calibrating VAD energy threshold, matching the Hammerspoon calibration UI.

**Design**:
- Interactive window (unlike preview window which is non-interactive)
- 3 steps: Background → Speech → Results
- Color-coded: Blue → Orange → Green
- Canvas-based drawing with click handlers

```go
package ui

import (
    "fmt"
    "math"
    "sort"
    "time"

    "github.com/progrium/macdriver/cocoa"
    "github.com/progrium/macdriver/core"
)

// CalibrationWindow manages the calibration wizard
type CalibrationWindow struct {
    nsWindow    cocoa.NSWindow
    canvas      cocoa.NSView  // For custom drawing
    step        int           // 1, 2, or 3
    isRecording bool

    // Stats from recordings
    backgroundStats *AudioStats
    speechStats     *AudioStats
    threshold       float64

    // Callbacks
    onRecord func(duration time.Duration) *AudioStats
    onSave   func(threshold float64) error
    onClose  func()
}

// AudioStats holds energy statistics from a recording
type AudioStats struct {
    Min float64
    Max float64
    Avg float64
    P5  float64
    P95 float64
}

// NewCalibrationWindow creates the calibration wizard
func NewCalibrationWindow() *CalibrationWindow {
    // Window: 500x400, centered
    frame := core.Rect(0, 0, 500, 400)

    style := cocoa.NSWindowStyleMaskTitled |
             cocoa.NSWindowStyleMaskClosable

    nsWindow := cocoa.NSWindow_Init(frame, style, cocoa.NSBackingStoreBuffered, false)
    nsWindow.SetTitle("VAD Calibration")
    nsWindow.Center()
    nsWindow.SetLevel(cocoa.NSFloatingWindowLevel)

    // Dark background
    nsWindow.SetBackgroundColor(cocoa.NSColor_ColorWithSRGBRed(0.15, 0.15, 0.15, 0.95))

    cw := &CalibrationWindow{
        nsWindow: nsWindow,
        step:     1,
    }

    // Setup close handler
    // Note: Would need to use NSWindowDelegate for proper close handling

    return cw
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
    // Clear and redraw content view with:
    // - Title: "🎤 VAD Calibration - Step 1/3"
    // - Subtitle: "Background Noise" (blue)
    // - Instructions: "Stay completely silent..."
    // - Button: "Start Recording" or "Recording... Xs"

    // Implementation would use NSTextField for text
    // and NSButton for the action button
    // Colors: Blue theme (#66ccff)
}

// drawStep2 renders the speech recording step
func (cw *CalibrationWindow) drawStep2() {
    // Similar to step 1 but:
    // - Subtitle: "Speech Recording" (orange)
    // - Instructions: "Speak normally and continuously..."
    // - Shows background stats from step 1
    // Colors: Orange theme (#ff9966)
}

// drawStep3 renders the results and save/cancel buttons
func (cw *CalibrationWindow) drawStep3() {
    // Shows:
    // - Title: "Results" (green)
    // - Background stats (Avg, P95)
    // - Speech stats (Avg, P5)
    // - Visual bars comparing the two
    // - Recommended threshold (large, green)
    // - "Save & Close" button (green)
    // - "Cancel" button (red)
    // Colors: Green theme (#66ff99)
}

// handleStartRecording initiates a recording for the current step
func (cw *CalibrationWindow) handleStartRecording() {
    if cw.isRecording || cw.onRecord == nil {
        return
    }

    cw.isRecording = true
    duration := 5 * time.Second

    // Update UI to show recording state
    if cw.step == 1 {
        cw.drawStep1()
    } else {
        cw.drawStep2()
    }

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
            // Show error notification
            // Could use NSAlert or system notification
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
```

**Button Click Handling**:

Since macdriver doesn't have direct button click callbacks like Hammerspoon canvas, we need to use NSButton with target-action:

```go
// Create button with action
func createButton(title string, frame core.NSRect, action func()) cocoa.NSButton {
    button := cocoa.NSButton_Init(frame)
    button.SetTitle(title)
    button.SetBezelStyle(cocoa.NSBezelStyleRounded)

    // For action handling, we need to use Objective-C runtime
    // to set target and action selector
    // This is more complex in macdriver - may need helper

    return button
}
```

**Alternative: Use NSAlert for simpler UI**:

For a simpler implementation, could use NSAlert dialogs for each step:

```go
// Simpler approach using alerts
func (cw *CalibrationWindow) runStep1Alert() {
    alert := cocoa.NSAlert_Init()
    alert.SetMessageText("Step 1: Background Noise")
    alert.SetInformativeText("Stay completely silent for 5 seconds.")
    alert.AddButtonWithTitle("Start Recording")
    alert.AddButtonWithTitle("Cancel")

    response := alert.RunModal()
    if response == cocoa.NSAlertFirstButtonReturn {
        // Start recording
    }
}
```

**Recording Implementation**:

The `onRecord` callback should:
1. Use existing audio capture from `client/internal/audio/`
2. Record for specified duration
3. Calculate energy for each 10ms frame
4. Return AudioStats

```go
// In ui.go, wire up the recording callback
func (u *UI) recordForCalibration(duration time.Duration) *AudioStats {
    // Create temporary audio capture
    capturer, err := audio.New(20, u.config.Audio.DeviceName, u.logger)
    if err != nil {
        return nil
    }
    defer capturer.Stop()

    var energies []float64
    done := make(chan struct{})

    // Collect audio chunks
    go func() {
        for chunk := range capturer.Chunks() {
            // Calculate energy for this chunk
            energy := calculateEnergy(chunk.Data)
            energies = append(energies, energy)
        }
        close(done)
    }()

    // Start capture
    capturer.Start()

    // Wait for duration
    time.Sleep(duration)

    // Stop capture
    capturer.Stop()
    <-done

    return CalculateAudioStats(energies)
}

// calculateEnergy computes RMS energy of audio samples
func calculateEnergy(samples []int16) float64 {
    var sum float64
    for _, s := range samples {
        sum += float64(s) * float64(s)
    }
    return math.Sqrt(sum / float64(len(samples)))
}
```

---

### 7. ui.go - Public Interface

**Purpose**: Clean public API that ties everything together, including calibration and permissions.

```go
package ui

import (
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
    window      *Window
    calibration *CalibrationWindow
    recording   bool
    mu          sync.Mutex

    config *config.Config
    logger *logger.ContextLogger

    // Callbacks
    onStart func() error
    onStop  func() error
}

// New creates a new UI instance
func New(cfg *config.Config, log *logger.Logger) *UI {
    return &UI{
        config: cfg,
        logger: log.With("ui"),
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
        WaitForAccessibilityPermissions(func(title, message string) {
            // Could show NSAlert here
            u.logger.Info("%s: %s", title, message)
        })
        u.logger.Info("Accessibility permissions granted!")
    }

    // Initialize clipboard
    if err := InitClipboard(); err != nil {
        panic("Failed to initialize clipboard: " + err.Error())
    }

    // Create calibration window (lazy, not shown yet)
    u.calibration = NewCalibrationWindow()
    u.calibration.SetHandlers(
        u.recordForCalibration,
        u.saveCalibrationThreshold,
        nil,
    )

    u.app = &App{
        onToggle:      u.toggleRecording,
        onCalibration: u.showCalibration,
    }

    u.app.Run() // Blocks forever
}

// showCalibration opens the calibration wizard
func (u *UI) showCalibration() {
    u.calibration.Show()
}

// recordForCalibration captures audio and returns energy stats
func (u *UI) recordForCalibration(duration time.Duration) *AudioStats {
    capturer, err := audio.New(20, u.config.Audio.DeviceName, u.logger.Logger)
    if err != nil {
        u.logger.Error("Failed to create audio capturer: %v", err)
        return nil
    }

    var energies []float64
    done := make(chan struct{})

    go func() {
        for chunk := range capturer.Chunks() {
            energy := calculateEnergy(chunk.Data)
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
    err := config.UpdateVADThreshold(u.config.FilePath(), threshold)
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
    // Clear previous text
    u.app.window.Clear()

    // Show window
    u.app.window.Show()

    // Call handler
    if u.onStart != nil {
        if err := u.onStart(); err != nil {
            // Show error via macOS notification
            showNotification("Recording Failed", err.Error())
            u.app.window.Hide()
            return
        }
    }

    u.recording = true
}

// showNotification displays a macOS notification for errors
// Implementation: use cocoa.NSUserNotification or osascript
func showNotification(title, message string) {
    // Option 1: Use NSUserNotificationCenter (deprecated but works)
    // Option 2: Shell out to osascript:
    // exec.Command("osascript", "-e",
    //     fmt.Sprintf(`display notification "%s" with title "%s"`, message, title)).Run()
}

func (u *UI) stopRecording() {
    // Get accumulated text
    text := u.app.window.GetText()

    // Call handler
    if u.onStop != nil {
        u.onStop()
    }

    // Paste text if we have any
    if text != "" {
        // Small delay to let things settle
        // Then paste
        go func() {
            // time.Sleep(100 * time.Millisecond)
            PasteText(text)
        }()
    }

    // Hide window
    u.app.window.Hide()

    u.recording = false
}

// AppendTranscription adds a transcription chunk (thread-safe)
func (u *UI) AppendTranscription(text string) {
    // Dispatch to main thread for UI update
    Dispatch(func() {
        u.app.window.AppendText(text)
    })
}

// IsRecording returns current recording state
func (u *UI) IsRecording() bool {
    u.mu.Lock()
    defer u.mu.Unlock()
    return u.recording
}
```

---

## Integration with main.go

### Modified main.go Structure

The main change is that the UI runs on the main thread and blocks. Audio capture and WebRTC run in goroutines.

```go
package main

import (
    // ... existing imports ...
    "github.com/lucianHymer/streaming-transcription/client/internal/ui"
)

// Global UI instance
var globalUI *ui.UI

func main() {
    // ... flag parsing and config loading (same as before) ...

    // Create logger
    log := logger.New(cfg.Client.Debug)

    // Create UI (checks accessibility permissions on startup)
    globalUI = ui.New(cfg, log)

    // Create WebRTC client
    webrtcClient := webrtc.New(cfg.Server.URL+"/api/v1/stream/signal", cfg, log, handleDataChannelMessage)

    // Connect in background
    go func() {
        if err := webrtcClient.Connect(); err != nil {
            log.Fatal("Failed to connect: %v", err)
        }
        // Wait for connection...
    }()

    // Create audio capturer
    capturer, err := audio.New(20, cfg.Audio.DeviceName, log)
    if err != nil {
        log.Fatal("Failed to create audio capturer: %v", err)
    }

    // Audio sending goroutine
    go func() {
        for chunk := range capturer.Chunks() {
            webrtcClient.SendAudioChunk(chunk.Data, chunk.SampleRate, chunk.Channels)
        }
    }()

    // Set UI handlers
    globalUI.SetHandlers(
        func() error {
            // Start recording
            webrtcClient.SendControlStart()
            return capturer.Start()
        },
        func() error {
            // Stop recording
            capturer.Stop()
            return webrtcClient.SendControlStop()
        },
    )

    // Run UI on main thread (blocks forever)
    globalUI.Run()
}

// handleDataChannelMessage - modified to update UI
func handleDataChannelMessage(msg *protocol.Message) {
    switch msg.Type {
    case protocol.MessageTypeTranscriptFinal:
        var transcript protocol.TranscriptData
        json.Unmarshal(msg.Data, &transcript)

        // Update UI
        if globalUI != nil {
            globalUI.AppendTranscription(transcript.Text)
        }

        // ... rest of handling (debug log, etc.) ...
    }
}
```

---

## What to Keep, Modify, Remove

### Keep Unchanged
- `server/` - Entire server codebase unchanged
- `client/internal/audio/` - Audio capture unchanged
- `client/internal/webrtc/` - WebRTC client unchanged
- `client/internal/config/` - Config system unchanged (but uses new UpdateVADThreshold)
- `client/internal/debuglog/` - Debug logging unchanged
- `shared/` - All shared code unchanged

### Modify
- `client/cmd/client/main.go` - Integrate UI, restructure main thread, remove API server

### Remove Entirely
- `hammerspoon/` - Entire directory (705 lines of Lua)
- `client/internal/api/` - Entire package (no longer needed)
- `client/internal/calibrate/` - CLI calibration (replaced by native UI)

---

## Dead Code Removal

**IMPORTANT**: This project should have NO dead code or unused dependencies after implementation. Clean up thoroughly.

### Directories to DELETE

```bash
# Delete Hammerspoon integration (705 lines of Lua)
rm -rf hammerspoon/

# Delete client API server package (no longer needed)
rm -rf client/internal/api/

# Delete CLI calibration package (replaced by native UI)
rm -rf client/internal/calibrate/
```

This removes:
- `hammerspoon/init.lua` (215 lines)
- `hammerspoon/calibration.lua` (490 lines)
- `hammerspoon/install.sh`
- `hammerspoon/README.md`
- `client/internal/api/server.go` (~400 lines) - HTTP server, WebSocket, calibration endpoints
- `client/internal/calibrate/calibrate.go` (~200 lines) - CLI calibration wizard

### Code to REMOVE from client/cmd/client/main.go

Remove ALL references to the API server:

1. **Import statement**:
```go
// DELETE this import
"github.com/lucianHymer/streaming-transcription/client/internal/api"
```

2. **Global API server variable**:
```go
// DELETE this variable
var globalAPIServer *api.Server
```

3. **API server creation and startup** (entire block):
```go
// DELETE all of this
apiServer := api.New(cfg, log, webrtcClient, debugLog)
apiServer.SetHandlers(...)
go func() {
    apiServer.Start()
}()
```

4. **All BroadcastTranscription calls** in handleDataChannelMessage:
```go
// DELETE these blocks
if globalAPIServer != nil {
    globalAPIServer.BroadcastTranscription(transcript.Text, false)
}
```

5. **API server shutdown**:
```go
// DELETE this line
apiServer.Stop()
```

6. **--calibrate flag handling** (replaced by native UI):
```go
// DELETE the calibrate flag and its handling
if *calibrateFlag {
    calibrate.Run(...)
    return
}
```

### Dependencies to REMOVE from go.mod

After removing API server and calibrate packages:
```bash
go mod tidy
```

This should remove `github.com/gorilla/websocket` since it's no longer used.

### Documentation to UPDATE

1. **README.md** - Remove Hammerspoon installation instructions, add native UI instructions
2. **Knowledge files** - Mark these as deprecated or archive them:
   - `.claude/knowledge/architecture/hammerspoon-integration.md`
   - `.claude/knowledge/architecture/vad-calibration-api.md`

### Verification Checklist

After cleanup, verify:
- [ ] `hammerspoon/` directory deleted
- [ ] `client/internal/api/` directory deleted
- [ ] `client/internal/calibrate/` directory deleted
- [ ] No `globalAPIServer` variable in main.go
- [ ] No `BroadcastTranscription` calls anywhere
- [ ] `go mod tidy` runs cleanly
- [ ] `go build` succeeds with CGO
- [ ] No imports of `github.com/gorilla/websocket` in client code
- [ ] `grep -r "hammerspoon" .` returns no results (except docs)
- [ ] `grep -r "api.Server" client/` returns no results
- [ ] Accessibility permissions check works on first run
- [ ] Calibration wizard (Ctrl+Alt+C) opens and works

---

## Testing Strategy

### Unit Tests
1. **Clipboard**: Write → Read → Verify
2. **Window**: Create → SetText → GetText → Verify
3. **AudioStats**: Calculate stats from sample energies

### Integration Tests
1. **Hotkey registration**: Manual test - press Ctrl+N and Ctrl+Alt+C, verify callbacks fire
2. **Window display**: Verify preview window shows without stealing focus
3. **Window non-interactive**: Verify preview window ignores mouse events during dictation
4. **Paste**: Copy text, paste into TextEdit, verify text appears
5. **Calibration window**: Verify calibration window is interactive (buttons work)

### End-to-End Test - Dictation
1. Start client
2. Open TextEdit
3. Press Ctrl+N
4. Speak
5. Press Ctrl+N
6. Verify: Window appeared, text was transcribed, text was pasted into TextEdit

### End-to-End Test - Calibration
1. Start client
2. Press Ctrl+Alt+C
3. Verify calibration window opens
4. Click "Start Recording" for background
5. Stay silent for 5 seconds
6. Click "Start Recording" for speech
7. Speak for 5 seconds
8. Verify threshold is calculated and displayed
9. Click "Save & Close"
10. Verify config file is updated with new threshold

### Accessibility Test
1. Remove app from Accessibility list in System Preferences
2. Start client → should show permission prompt
3. Grant permissions → should continue automatically
4. Paste should work after permissions granted

### First Run Experience
1. Fresh install (no existing permissions)
2. Launch app → should prompt for accessibility
3. System Preferences should open to Accessibility pane
4. Add app to list → app should detect and continue
5. Hotkeys should work immediately after

---

## Potential Gotchas

### 1. Main Thread Requirement
**Problem**: macOS UI must run on main thread, but Go's main goroutine isn't necessarily the main thread.

**Solution**: Use `runtime.LockOSThread()` at the start of main(), OR just let macdriver handle it (it does internally).

### 2. CGO Build Complexity
**Problem**: The hotkey and paste code use CGO with Objective-C.

**Solution**: Make sure build flags are correct. Test build early.

```bash
CGO_ENABLED=1 go build ./client/cmd/client
```

### 3. Accessibility Permissions
**Problem**: Cmd+V simulation requires accessibility permissions.

**Solution**: Document this requirement. On first run, the app will be blocked until permissions are granted. Consider showing a helpful error message.

### 4. Window Focus and Interaction Modes
**Problem**: Preview window needs to be non-interactive (click-through), but calibration window needs to be interactive (buttons).

**Solution**:
- **Preview window**: `SetIgnoresMouseEvents(true)` - clicks pass through to app below
- **Calibration window**: `SetIgnoresMouseEvents(false)` (default) - buttons work normally

These are two different NSWindow instances with different interaction modes.

### 5. Text Accumulation
**Problem**: Need to accumulate transcription chunks correctly with spaces.

**Solution**: The `AppendText` method handles this by adding a space between chunks.

### 6. Dispatch from Wrong Thread
**Problem**: Updating UI from WebRTC callback goroutine will crash.

**Solution**: Always use `ui.Dispatch()` for UI updates from non-main goroutines.

---

## Build Commands

### Development Build
```bash
cd /workspace/project
CGO_ENABLED=1 go build -o client/cmd/client/client ./client/cmd/client
```

### With RNNoise (if needed for testing)
```bash
. ./scripts/setup-env.sh
go build -tags rnnoise -o client/cmd/client/client ./client/cmd/client
```

### Run
```bash
./client/cmd/client/client --config ~/.config/richardtate/client.yaml
```

---

## Future Enhancements (Out of Scope)

These are NOT part of this implementation but could be added later:

1. **Native calibration UI** - Replace CLI calibration with native wizard
2. **Configurable hotkey** - Let user change from Ctrl+N
3. **Menu bar icon** - Show status in menu bar
4. **Settings window** - Native preferences UI
5. **Multiple windows** - Preview + history

---

## Timeline Estimate

- **Day 1**: Set up macdriver, basic window, verify no focus stealing, non-interactive mode
- **Day 2**: Hotkey registration (both Ctrl+N and Ctrl+Alt+C), clipboard + paste
- **Day 3**: Accessibility permissions check/prompt flow
- **Day 4**: Calibration wizard UI (3 steps, buttons, recording)
- **Day 5**: Integration with main.go, wire up transcription + calibration
- **Day 6**: Testing, bug fixes, edge cases, first run experience
- **Day 7**: Dead code removal, documentation, cleanup, PR

**Total: ~5-7 days** (increased from original 3-5 due to calibration UI and permissions)

---

## Resources

- **macdriver docs**: https://github.com/progrium/macdriver
- **macdriver examples**: https://github.com/progrium/macdriver/tree/main/examples
- **Carbon key codes**: https://gist.github.com/eegrok/949034
- **CGEventPost docs**: https://developer.apple.com/documentation/coregraphics/1456564-cgeventpost
- **golang.design/x/clipboard**: https://pkg.go.dev/golang.design/x/clipboard

---

## Contact

For questions about:
- **Existing codebase**: Check the knowledge files in `.claude/knowledge/`
- **Architecture decisions**: See `docs/` folder
- **WebRTC/Audio**: `client/internal/webrtc/` and `client/internal/audio/`
- **Server transcription**: `server/internal/transcription/`
