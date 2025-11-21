# Richard Tate Native macOS UI

Native Swift subprocess for displaying transcription UI on macOS.

## Building

```bash
swift build -c release
```

The binary will be at `.build/release/richardtate-ui`.

## Usage

The UI process reads JSON commands from stdin and manages two windows:

### Preview Window
Shows real-time transcription during recording.

Commands:
- `{"command": "show"}` - Show window
- `{"command": "hide"}` - Hide window
- `{"command": "setText", "text": "..."}` - Update text
- `{"command": "setProcessing", "processing": true}` - Show/hide "..." indicator
- `{"command": "clearText"}` - Clear text

### Calibration Window
3-step wizard for VAD threshold calibration.

Commands:
- `{"command": "showCalibration"}` - Show calibration window
- `{"command": "hideCalibration"}` - Hide calibration window
- `{"command": "setCalibrationStep", "step": 1}` - Set step (1-3)
- `{"command": "setCalibrationMessage", "message": "..."}` - Update message
- `{"command": "setCalibrationProgress", "value": 0.5}` - Update progress (0.0-1.0)
- `{"command": "setCalibrationStats", "backgroundP95": 78.1, "speechP5": 290.2, "recommended": 184.2}` - Show results

### Lifecycle
- `{"command": "quit"}` - Exit process

## Testing

You can test the UI manually:

```bash
# Build
swift build -c release

# Run and send commands
.build/release/richardtate-ui

# In another terminal, send commands:
echo '{"command": "show"}' | nc localhost <stdin>
```

Or use a script:
```bash
#!/bin/bash
exec .build/release/richardtate-ui << 'EOF'
{"command": "show"}
{"command": "setText", "text": "Hello from Swift UI!"}
{"command": "setProcessing", "processing": true}
EOF
```
