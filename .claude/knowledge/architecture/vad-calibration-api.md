# VAD Calibration API Architecture

**Status**: ✅ Implemented (Native macOS UI)
**Last Updated**: 2025-11-21

## Overview
VAD calibration system using native macOS UI built with DarwinKit. Provides visual 3-step wizard for calibrating energy threshold based on environment noise and speech patterns.

## Implementation Status

### ✅ Completed
- Native macOS calibration wizard (DarwinKit)
- 3-step UI: Background recording → Speech recording → Results
- Threshold calculation with P95 × 1.5 logic
- Direct config file save with hot-reload
- RNNoise processing in calibration (matches production)

### 📋 Future Enhancements
- Real-time energy level visualization during recording
- Multiple calibration profiles (home, office, etc.)
- Calibration history tracking

## Benefits

### 1. Native UI Calibration
- Native macOS wizard using DarwinKit (Ctrl+Alt+C hotkey)
- Visual progress indicators during recording
- Energy comparison bars for background vs speech
- One-click save to config with hot-reload

### 2. Real-Time Visual Feedback
- Energy level meters during recording
- Live speech/silence detection visualization
- Better user understanding of thresholds
- Immediate feedback on environment noise

### 3. No Client Restart Required
- Calibration runs while client is already running
- Seamless integration with existing session
- Settings apply immediately
- Better user workflow

### 4. Better User Experience
- Visual progress indicators
- Interactive threshold adjustment
- Test mode to verify settings
- Multiple calibration profiles

## Architecture Rationale

### Why Client-Side (Not Server)
Calibration runs entirely on the client:

1. **Direct Microphone Access**: Client has exclusive access to audio hardware
2. **Stateful UI**: Wizard manages multi-step flow with visual feedback
3. **Privacy**: Audio never leaves the device during calibration
4. **Local Configuration**: Results save directly to client config file
5. **No Network Dependency**: Works even if server is unreachable

### Integration Points
- Reuses `client/internal/audio/capture.go` for recording
- Uses `client/internal/ui/calibration.go` for wizard UI
- Updates config via `client/internal/config/update.go`
- No server communication required

## Native macOS UI Integration

### Visual Calibration Wizard
**Status**: ✅ Implemented
**Location**: `client/internal/ui/calibration.go`

Native DarwinKit-based calibration wizard for macOS:

**Design Features**:
- 3-step wizard: Background → Speech → Results
- Native NSWindow with AppKit controls
- Dark theme matching macOS aesthetic
- Real-time progress bars (NSProgressIndicator)
- Visual energy comparison bars
- Native button controls (Save/Cancel)

**User Flow**:
1. Press **Ctrl+Alt+C** to open calibration wizard (global hotkey via Carbon Events)
2. **Step 1** (Blue theme): "Stay silent" → Records 5s background → Shows stats
3. **Step 2** (Orange theme): "Speak normally" → Records 5s speech → Shows stats
4. **Step 3** (Green theme): Visual bars → Recommended threshold → Save/Cancel buttons

**Why Native UI (Not Hammerspoon)**:
- No external dependencies (Hammerspoon not required)
- Faster text insertion via clipboard (no keystroke simulation)
- Better macOS integration and reliability
- Single Go binary deployment

**Implementation**:
- Hotkey: Ctrl+Alt+C via Carbon Events CGO
- Direct config file updates via `config.UpdateVADThreshold()`
- Hot-reload support (no client restart needed)
- Thread-safe dispatch to main thread for UI updates

**Files**:
- `client/internal/ui/calibration.go` - Wizard implementation
- `client/internal/ui/hotkey.go` - Global hotkey registration
- `client/internal/ui/ui.go` - Integration and handlers

## Implementation Details

### Native UI Architecture
The calibration wizard is integrated directly into the client's native UI layer:

**Initialization**:
```go
// In ui.go
ui.app.SetCalibrationHandlers(
    ui.recordForCalibration,
    ui.saveCalibrationThreshold,
)
```

**Recording Handler**:
- Creates temporary audio capturer
- Captures for specified duration
- Calculates energy statistics locally
- Returns AudioStats struct

**Save Handler**:
- Calls `config.UpdateVADThreshold()` to update YAML
- Triggers `config.Reload()` for hot-reload
- No client restart required

### Energy Calculation
**Implementation**: Client calculates energy locally using same algorithm as server VAD

**Process**:
1. Audio captured in 10ms frames (160 samples at 16kHz)
2. RMS energy calculated per frame: `sqrt(sum(sample²) / frameLen)`
3. Statistics computed: min, max, avg, p5, p95
4. Threshold calculated: `background_p95 × 1.5`

**Algorithm Match**:
- Same frame size as server VAD (10ms / 160 samples)
- Same RMS energy formula
- Ensures calibrated threshold works correctly in production

### Local Calculation Design
**Key Decision**: All calibration logic runs locally in the client

**Benefits**:
1. **No network dependency**: Works even if server is down
2. **Simpler architecture**: No API endpoints needed
3. **Faster**: No network round-trip for stats calculation
4. **Privacy**: Audio never leaves the device during calibration

**Calculation Logic**:
- Background and speech recording done locally
- Energy calculation uses same algorithm as server VAD
- Threshold = `background_p95 × 1.5`
- All logic in `ui/calibration.go` and `ui/ui.go`

## Performance Characteristics

### Latency
- **Record 5s audio**: ~5 seconds + 100-200ms processing
- **Calculate threshold**: < 10ms (simple math)
- **Save to config**: < 50ms (YAML parsing + write)
- **Total wizard flow**: ~15-20 seconds (10s recording + UI transitions)

### Resource Usage
- **Temporary RNNoise processor**: Created per-request, cleaned up immediately
- **Audio buffer**: 5 seconds at 16kHz = 160KB max
- **Memory overhead**: Minimal (~200KB per calibration session)

### Reliability
- **Config backup**: YAML parser preserves structure and comments
- **Error recovery**: Graceful error handling with macOS alerts
- **UI state**: Properly cleanup on wizard close
- **Hot-reload**: Config changes apply immediately without restart

## Known Limitations

1. **No real-time streaming**: WebSocket endpoint not yet implemented
2. **Single profile**: Can't save multiple calibration profiles (home, office, etc.)
3. **Fixed recording duration**: Hardcoded to 5 seconds
4. **No validation mode**: Can't test current threshold before saving

## Related Systems
- [VAD Calibration Workflow](../workflows/vad-calibration.md) - CLI implementation details
- [Native macOS UI](../architecture/native-macos-ui.md) - Main recording UI and architecture
- [Per-Client Pipeline](../architecture/per-client-pipeline.md) - How calibrated settings are used
- [Transcription Gotchas](../gotchas/transcription-gotchas.md#vad-calibration-missing-rnnoise-processing) - RNNoise processing fix