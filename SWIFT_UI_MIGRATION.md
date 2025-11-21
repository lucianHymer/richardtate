# Swift Native UI Migration Plan

**Status**: Proposal
**Date**: 2025-11-21
**Reason**: Replace DarwinKit-based UI with native Swift subprocess to eliminate GC segfaults

## Problem Statement

The current DarwinKit-based UI (`client/internal/ui/`) experiences repeated segmentation faults (18 crashes logged) due to conflicts between Go's garbage collection and Objective-C reference counting. The window recreation pattern (used to work around show/hide reliability issues) causes old windows to be finalized by Go's GC, leading to crashes when finalizers call `Release()` on deallocated Objective-C objects.

**Error signature**:
```
SIGSEGV: segmentation violation
PC=0x18d154028 m=5 sigcode=2 addr=0x20
signal arrived during cgo execution
github.com/progrium/darwinkit/objc.Object.Release
```

## Proposed Solution

Replace the DarwinKit UI with a **native Swift subprocess** that communicates via JSON over stdin/stdout. This eliminates:
- CGO overhead and complexity
- Go GC finalization issues
- DarwinKit dependency (~5MB+ of code)
- Objective-C bridge bugs

## Architecture

```
┌─────────────────────┐         JSON over       ┌──────────────────────┐
│                     │        stdin/stdout      │                      │
│  Go Client Process  │ ◄────────────────────► │  Swift UI Process    │
│                     │                          │                      │
│ - WebRTC            │                          │ - NSWindow           │
│ - Audio capture     │                          │ - NSTextField        │
│ - Transcription     │                          │ - Progress bars      │
│ - Hotkeys           │                          │ - Native Cocoa UI    │
└─────────────────────┘                          └──────────────────────┘
```

## IPC Protocol

### Commands (Go → Swift)

#### Preview Window Commands
```json
{"command": "show"}
{"command": "hide"}
{"command": "setText", "text": "transcribed text here"}
{"command": "setProcessing", "processing": true}
{"command": "clearText"}
```

#### Calibration Window Commands
```json
{"command": "showCalibration"}
{"command": "hideCalibration"}
{"command": "setCalibrationStep", "step": 1}
{"command": "setCalibrationMessage", "message": "Stay silent..."}
{"command": "setCalibrationProgress", "value": 0.5}
{"command": "setCalibrationStats", "backgroundP95": 78.1, "speechP5": 290.2, "recommended": 184.2}
```

#### Lifecycle Commands
```json
{"command": "quit"}
```

### Events (Swift → Go) - Optional

If needed in the future (currently not required):
```json
{"event": "calibrationSave", "threshold": 184.2}
{"event": "calibrationCancel"}
```

## Implementation

### Part 1: Swift UI Binary

**Location**: `client/ui-macos/` (new directory)

#### Files Structure
```
client/ui-macos/
├── Package.swift              # Swift package manifest
├── Sources/
│   └── richardtate-ui/
│       ├── main.swift         # Entry point, stdin reader
│       ├── PreviewWindow.swift    # Floating transcription window
│       └── CalibrationWindow.swift # Calibration wizard
└── README.md                  # Build instructions
```

#### PreviewWindow.swift

```swift
import Cocoa

class PreviewWindow: NSWindow {
    private let textField: NSTextField
    private var displayText: String = ""
    private var isProcessing: Bool = false

    init() {
        // Window configuration
        let frame = NSRect(x: 0, y: 0, width: 400, height: 200)
        super.init(
            contentRect: frame,
            styleMask: [.titled, .resizable],  // No close button
            backing: .buffered,
            defer: false
        )

        // Window properties
        self.title = "Dictation Preview"
        self.level = .floating
        self.backgroundColor = NSColor(red: 0.1, green: 0.1, blue: 0.1, alpha: 0.95)
        self.isMovableByWindowBackground = true
        self.ignoresMouseEvents = true  // Click-through
        self.center()

        // Create text field (wrapping label)
        textField = NSTextField(wrappingLabelWithString: "")
        textField.frame = NSRect(x: 10, y: 10, width: 380, height: 180)
        textField.textColor = NSColor(red: 0.4, green: 0.95, blue: 0.7, alpha: 1.0)
        textField.font = NSFont.systemFont(ofSize: 14)
        textField.alignment = .left

        contentView?.addSubview(textField)
    }

    func setText(_ text: String) {
        displayText = text
        updateDisplay()
    }

    func setProcessing(_ processing: Bool) {
        isProcessing = processing
        updateDisplay()
    }

    func clearText() {
        displayText = ""
        isProcessing = false
        updateDisplay()
    }

    private func updateDisplay() {
        var display = displayText

        // Append "..." if processing
        if isProcessing {
            display += "..."
        }

        // Limit to last 500 characters
        if display.count > 500 {
            display = "..." + String(display.suffix(497))
        }

        textField.stringValue = display
    }
}
```

#### CalibrationWindow.swift

```swift
import Cocoa

class CalibrationWindow: NSWindow {
    private let messageLabel: NSTextField
    private let progressIndicator: NSProgressIndicator
    private let statsLabel: NSTextField
    private let backgroundBar: NSView
    private let speechBar: NSView
    private var currentStep: Int = 0

    init() {
        // Window configuration
        let frame = NSRect(x: 0, y: 0, width: 500, height: 400)
        super.init(
            contentRect: frame,
            styleMask: [.titled],  // No close button
            backing: .buffered,
            defer: false
        )

        self.title = "VAD Calibration"
        self.center()

        // Create UI elements
        messageLabel = NSTextField(labelWithString: "")
        messageLabel.frame = NSRect(x: 20, y: 320, width: 460, height: 60)
        messageLabel.font = NSFont.systemFont(ofSize: 16)
        messageLabel.alignment = .center

        progressIndicator = NSProgressIndicator()
        progressIndicator.frame = NSRect(x: 100, y: 280, width: 300, height: 20)
        progressIndicator.style = .bar
        progressIndicator.minValue = 0
        progressIndicator.maxValue = 1
        progressIndicator.isIndeterminate = false

        statsLabel = NSTextField(wrappingLabelWithString: "")
        statsLabel.frame = NSRect(x: 20, y: 100, width: 460, height: 160)
        statsLabel.font = NSFont.monospacedSystemFont(ofSize: 12, weight: .regular)

        backgroundBar = NSView(frame: NSRect(x: 100, y: 60, width: 0, height: 20))
        backgroundBar.wantsLayer = true
        backgroundBar.layer?.backgroundColor = NSColor.systemBlue.cgColor

        speechBar = NSView(frame: NSRect(x: 100, y: 30, width: 0, height: 20))
        speechBar.wantsLayer = true
        speechBar.layer?.backgroundColor = NSColor.systemGreen.cgColor

        contentView?.addSubview(messageLabel)
        contentView?.addSubview(progressIndicator)
        contentView?.addSubview(statsLabel)
        contentView?.addSubview(backgroundBar)
        contentView?.addSubview(speechBar)
    }

    func setStep(_ step: Int) {
        currentStep = step

        // Reset visibility for step transitions
        progressIndicator.isHidden = false
        statsLabel.isHidden = true
        backgroundBar.isHidden = true
        speechBar.isHidden = true

        switch step {
        case 1:
            self.backgroundColor = NSColor(red: 0.2, green: 0.4, blue: 0.8, alpha: 1.0)
        case 2:
            self.backgroundColor = NSColor(red: 0.8, green: 0.5, blue: 0.2, alpha: 1.0)
        case 3:
            self.backgroundColor = NSColor(red: 0.2, green: 0.7, blue: 0.3, alpha: 1.0)
            progressIndicator.isHidden = true
            statsLabel.isHidden = false
            backgroundBar.isHidden = false
            speechBar.isHidden = false
        default:
            break
        }
    }

    func setMessage(_ message: String) {
        messageLabel.stringValue = message
    }

    func setProgress(_ value: Double) {
        progressIndicator.doubleValue = value
    }

    func setStats(backgroundP95: Double, speechP5: Double, recommended: Double) {
        let stats = """
        Analysis Results:

        Background Noise P95: \(String(format: "%.1f", backgroundP95))
        Speech P5: \(String(format: "%.1f", speechP5))

        Recommended Threshold: \(String(format: "%.1f", recommended))
        """
        statsLabel.stringValue = stats

        // Update bar widths (scale to 300px max)
        let maxWidth: CGFloat = 300
        let scale = maxWidth / max(backgroundP95, speechP5)
        backgroundBar.frame.size.width = CGFloat(backgroundP95) * scale
        speechBar.frame.size.width = CGFloat(speechP5) * scale
    }
}
```

#### main.swift

```swift
import Cocoa
import Foundation

class AppDelegate: NSObject, NSApplicationDelegate {
    var previewWindow: PreviewWindow!
    var calibrationWindow: CalibrationWindow!

    func applicationDidFinishLaunching(_ notification: Notification) {
        // Create windows (initially hidden)
        previewWindow = PreviewWindow()
        calibrationWindow = CalibrationWindow()

        // Start reading commands from stdin
        startCommandLoop()
    }

    func startCommandLoop() {
        DispatchQueue.global(qos: .userInitiated).async {
            while let line = readLine() {
                guard let data = line.data(using: .utf8),
                      let json = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
                      let command = json["command"] as? String else {
                    continue
                }

                // Execute on main thread (AppKit requirement)
                DispatchQueue.main.async {
                    self.handleCommand(command, json: json)
                }
            }

            // stdin closed, exit
            NSApplication.shared.terminate(nil)
        }
    }

    func handleCommand(_ command: String, json: [String: Any]) {
        switch command {
        // Preview window commands
        case "show":
            previewWindow.makeKeyAndOrderFront(nil)
        case "hide":
            previewWindow.orderOut(nil)
        case "setText":
            if let text = json["text"] as? String {
                previewWindow.setText(text)
            }
        case "setProcessing":
            if let processing = json["processing"] as? Bool {
                previewWindow.setProcessing(processing)
            }
        case "clearText":
            previewWindow.clearText()

        // Calibration window commands
        case "showCalibration":
            calibrationWindow.makeKeyAndOrderFront(nil)
        case "hideCalibration":
            calibrationWindow.orderOut(nil)
        case "setCalibrationStep":
            if let step = json["step"] as? Int {
                calibrationWindow.setStep(step)
            }
        case "setCalibrationMessage":
            if let message = json["message"] as? String {
                calibrationWindow.setMessage(message)
            }
        case "setCalibrationProgress":
            if let value = json["value"] as? Double {
                calibrationWindow.setProgress(value)
            }
        case "setCalibrationStats":
            if let bg = json["backgroundP95"] as? Double,
               let sp = json["speechP5"] as? Double,
               let rec = json["recommended"] as? Double {
                calibrationWindow.setStats(
                    backgroundP95: bg,
                    speechP5: sp,
                    recommended: rec
                )
            }

        // Lifecycle
        case "quit":
            NSApplication.shared.terminate(nil)

        default:
            print("Unknown command: \(command)", to: &standardError)
        }
    }
}

// Helper for stderr output
var standardError = FileHandle.standardError

extension FileHandle: TextOutputStream {
    public func write(_ string: String) {
        guard let data = string.data(using: .utf8) else { return }
        self.write(data)
    }
}

// Main entry point
let app = NSApplication.shared
let delegate = AppDelegate()
app.delegate = delegate
app.run()
```

#### Package.swift

```swift
// swift-tools-version: 5.9
import PackageDescription

let package = Package(
    name: "richardtate-ui",
    platforms: [.macOS(.v11)],
    products: [
        .executable(name: "richardtate-ui", targets: ["richardtate-ui"])
    ],
    targets: [
        .executableTarget(name: "richardtate-ui")
    ]
)
```

### Part 2: Go Integration

**Location**: `client/internal/swiftui/` (new package)

#### swiftui.go

```go
package swiftui

import (
    "bufio"
    "encoding/json"
    "fmt"
    "io"
    "os/exec"
    "sync"
)

// Window manages the Swift UI subprocess
type Window struct {
    cmd    *exec.Cmd
    stdin  io.WriteCloser
    stdout io.ReadCloser
    stderr io.ReadCloser
    mu     sync.Mutex
}

// NewWindow spawns the Swift UI process
func NewWindow(binaryPath string) (*Window, error) {
    cmd := exec.Command(binaryPath)

    stdin, err := cmd.StdinPipe()
    if err != nil {
        return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
    }

    stdout, err := cmd.StdoutPipe()
    if err != nil {
        return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
    }

    stderr, err := cmd.StderrPipe()
    if err != nil {
        return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
    }

    if err := cmd.Start(); err != nil {
        return nil, fmt.Errorf("failed to start UI process: %w", err)
    }

    w := &Window{
        cmd:    cmd,
        stdin:  stdin,
        stdout: stdout,
        stderr: stderr,
    }

    // Monitor stderr for errors
    go w.monitorStderr()

    return w, nil
}

// monitorStderr logs any errors from the Swift process
func (w *Window) monitorStderr() {
    scanner := bufio.NewScanner(w.stderr)
    for scanner.Scan() {
        fmt.Printf("[Swift UI Error] %s\n", scanner.Text())
    }
}

// sendCommand sends a JSON command to the Swift UI
func (w *Window) sendCommand(cmd map[string]interface{}) error {
    w.mu.Lock()
    defer w.mu.Unlock()

    data, err := json.Marshal(cmd)
    if err != nil {
        return err
    }

    _, err = w.stdin.Write(append(data, '\n'))
    return err
}

// Show displays the preview window
func (w *Window) Show() error {
    return w.sendCommand(map[string]interface{}{"command": "show"})
}

// Hide hides the preview window
func (w *Window) Hide() error {
    return w.sendCommand(map[string]interface{}{"command": "hide"})
}

// SetText updates the preview text
func (w *Window) SetText(text string) error {
    return w.sendCommand(map[string]interface{}{
        "command": "setText",
        "text":    text,
    })
}

// SetProcessing updates the processing indicator
func (w *Window) SetProcessing(processing bool) error {
    return w.sendCommand(map[string]interface{}{
        "command":    "setProcessing",
        "processing": processing,
    })
}

// ClearText clears the preview text
func (w *Window) ClearText() error {
    return w.sendCommand(map[string]interface{}{"command": "clearText"})
}

// ShowCalibration shows the calibration window
func (w *Window) ShowCalibration() error {
    return w.sendCommand(map[string]interface{}{"command": "showCalibration"})
}

// HideCalibration hides the calibration window
func (w *Window) HideCalibration() error {
    return w.sendCommand(map[string]interface{}{"command": "hideCalibration"})
}

// SetCalibrationStep sets the calibration wizard step
func (w *Window) SetCalibrationStep(step int) error {
    return w.sendCommand(map[string]interface{}{
        "command": "setCalibrationStep",
        "step":    step,
    })
}

// SetCalibrationMessage sets the calibration message
func (w *Window) SetCalibrationMessage(message string) error {
    return w.sendCommand(map[string]interface{}{
        "command": "setCalibrationMessage",
        "message": message,
    })
}

// SetCalibrationProgress updates the progress bar (0.0 to 1.0)
func (w *Window) SetCalibrationProgress(value float64) error {
    return w.sendCommand(map[string]interface{}{
        "command": "setCalibrationProgress",
        "value":   value,
    })
}

// SetCalibrationStats displays the final calibration results
func (w *Window) SetCalibrationStats(backgroundP95, speechP5, recommended float64) error {
    return w.sendCommand(map[string]interface{}{
        "command":       "setCalibrationStats",
        "backgroundP95": backgroundP95,
        "speechP5":      speechP5,
        "recommended":   recommended,
    })
}

// Close terminates the Swift UI process
func (w *Window) Close() error {
    // Send quit command
    w.sendCommand(map[string]interface{}{"command": "quit"})

    // Wait for process to exit
    return w.cmd.Wait()
}
```

### Part 3: Migration Steps

#### Step 1: Build Swift UI Binary
```bash
cd client/ui-macos
swift build -c release
cp .build/release/richardtate-ui ../../bin/
```

Add to `.gitignore`:
```
bin/richardtate-ui
```

#### Step 2: Update Main Client

**client/cmd/client/main.go**:

Replace DarwinKit UI initialization with:
```go
import (
    "github.com/lucianHymer/streaming-transcription/client/internal/swiftui"
)

func main() {
    // ... existing config loading ...

    // Initialize Swift UI
    uiBinaryPath := "./bin/richardtate-ui"  // Adjust path as needed
    window, err := swiftui.NewWindow(uiBinaryPath)
    if err != nil {
        log.Fatal("Failed to start UI: %v", err)
    }
    defer window.Close()

    // ... rest of initialization ...

    // Replace old ui.Run() with:
    // - Register hotkeys (keep existing Carbon Events code)
    // - Start WebRTC client
    // - Block forever (select{} or similar)
}
```

#### Step 3: Update Message Handlers

Where you currently call:
```go
ui.app.GetWindow().SetText(text)
ui.app.GetWindow().SetProcessing(true)
```

Replace with:
```go
window.SetText(text)
window.SetProcessing(true)
```

#### Step 4: Update Calibration

Where you currently show calibration wizard:
```go
ui.app.GetCalibration().Show()
```

Replace with:
```go
window.ShowCalibration()
window.SetCalibrationStep(1)
window.SetCalibrationMessage("Stay silent for 5 seconds...")
// ... update progress during recording ...
window.SetCalibrationProgress(0.5)
// ... show final results ...
window.SetCalibrationStats(backgroundP95, speechP5, recommended)
```

#### Step 5: Remove DarwinKit Dependencies

```bash
# Remove old UI package
rm -rf client/internal/ui/

# Update go.mod
go mod tidy  # Will remove unused DarwinKit dependencies
```

#### Step 6: Update Build Scripts

**scripts/build-mac.sh** - Add Swift UI build step:

The existing script already handles:
- Whisper.cpp detection and CGO setup
- RNNoise detection and build tag
- Parakeet MLX installation
- Client and server builds

**Add this section after line 208 (before "Build server" section)**:

```bash
# Build Swift UI
echo ""
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Building Swift UI..."
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"

if [ -d "client/ui-macos" ]; then
    cd client/ui-macos
    if swift build -c release; then
        echo "✓ Swift UI built successfully"
        mkdir -p ../../bin
        cp .build/release/richardtate-ui ../../bin/
        echo "✓ Swift UI binary copied to bin/"
    else
        echo "✗ Swift UI build failed"
        exit 1
    fi
    cd ../..
else
    echo "⚠ Swift UI directory not found (client/ui-macos)"
    echo "  Skipping Swift UI build"
fi
```

**Also update the final summary** at the end of the script to include:
```bash
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo "Build Complete!"
echo "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━"
echo ""
echo "Built with:"
echo "  - Whisper.cpp: $WHISPER_STATUS"
echo "  - RNNoise: $RNNOISE_STATUS"
echo "  - Parakeet MLX: $PARAKEET_STATUS"
echo "  - Swift UI: $([ -f bin/richardtate-ui ] && echo '✓ enabled' || echo '✗ not found')"
echo ""
echo "Client binary: bin/richardtate"
echo "Server binary: bin/richardtate-server"
echo "Swift UI binary: bin/richardtate-ui"
```

**Note**: The script should gracefully handle the case where `client/ui-macos` doesn't exist yet (during migration), so it's safe to add this even before implementing the Swift UI.

## Benefits

### Reliability
- **No GC segfaults** - Swift uses ARC, perfectly compatible with Cocoa
- **No CGO bugs** - Pure Go + pure Swift, no bridge
- **Process isolation** - UI crashes don't kill transcription process

### Performance
- **No CGO overhead** - Main thread not locked by Objective-C calls
- **Smaller binary** - Remove ~5MB DarwinKit dependency
- **Faster startup** - No DarwinKit initialization

### Maintainability
- **Simpler debugging** - Swift crashes have real stack traces
- **Better docs** - Swift/Cocoa documentation is excellent
- **Easier to fix** - Own the UI code, no DarwinKit black box

## Migration Checklist

- [ ] Create `client/ui-macos/` directory structure
- [ ] Implement `PreviewWindow.swift`
- [ ] Implement `CalibrationWindow.swift`
- [ ] Implement `main.swift` with command loop
- [ ] Create `Package.swift`
- [ ] Build Swift binary and test standalone
- [ ] Create `client/internal/swiftui/` package
- [ ] Implement Go wrapper with IPC
- [ ] Keep existing hotkey registration (Carbon Events CGO - this stays)
- [ ] Update main.go to use Swift UI
- [ ] Update message handlers
- [ ] Update calibration flow
- [ ] Test end-to-end
- [ ] Remove `client/internal/ui/` (DarwinKit code)
- [ ] Update build scripts
- [ ] Update documentation

## Testing Plan

### Unit Tests
- Swift UI: Test command parsing and window state
- Go wrapper: Test IPC message formatting

### Integration Tests
1. Launch UI subprocess, verify it starts
2. Send `show` command, verify window appears
3. Send `setText`, verify text updates
4. Send `setProcessing`, verify "..." appears
5. Test calibration flow with all 3 steps
6. Test `quit` command, verify clean shutdown

### Edge Cases
- UI process crashes → Go process should continue, log error
- Go process crashes → UI process exits (stdin closes)
- Rapid text updates → Should not drop messages
- Long text (>500 chars) → Should truncate correctly

## Rollback Plan

If issues arise:
1. Keep old `client/internal/ui/` in a branch
2. Can revert to DarwinKit version
3. Swift UI is additive - doesn't break existing code until fully migrated

## Open Questions

1. **UI binary distribution**: Should we embed the Swift binary in the Go binary, or distribute separately?
   - **Recommendation**: Separate for now, embed later if needed

2. **Hotkey handling**: Keep in Go (Carbon Events) or move to Swift?
   - **Recommendation**: Keep in Go - it already works, no reason to move

3. **Multiple windows**: Should we support multiple preview windows (future)?
   - **Recommendation**: No, out of scope for this migration

## Estimated Timeline

- **Swift implementation**: 2-3 hours
- **Go integration**: 1 hour
- **Testing**: 1-2 hours
- **Documentation**: 30 minutes
- **Total**: 4-6 hours

## References

- Current DarwinKit implementation: `client/internal/ui/`
- IPC inspiration: parakeet_worker.py (JSON over stdin/stdout)
- Swift/Cocoa docs: https://developer.apple.com/documentation/appkit
