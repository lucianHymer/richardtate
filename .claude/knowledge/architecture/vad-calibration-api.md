# VAD Calibration API Architecture

**Status**: ✅ Implemented
**Last Updated**: 2025-11-06

## Overview
Complete API-based VAD calibration system enabling UI-driven calibration through client daemon endpoints. Supports both terminal CLI and Hammerspoon visual wizards without requiring client restart.

## Implementation Status

### ✅ Completed
- Client API endpoints for recording and calculation
- Stateless recording endpoint (Hammerspoon controls phase)
- Threshold calculation endpoint with P95 × 1.5 logic
- Config save endpoint with YAML updates
- Hammerspoon visual calibration wizard
- RNNoise processing in calibration (matches production)

### 📋 Future Enhancements
- WebSocket endpoint for real-time energy streaming
- Web UI calibration wizard
- Multiple calibration profiles

## API Architecture

### Client-Side Endpoints
Calibration exposed as HTTP endpoints on client daemon (localhost:8081):

#### POST /api/calibrate/record
**Status**: ✅ Implemented

Records audio for specified duration and returns energy statistics.

**Key Design**: Stateless - endpoint doesn't know if it's recording background or speech. UI (Hammerspoon/CLI) decides the phase and interprets results.

**Request**:
```json
{
  "duration_seconds": 5
}
```

**Response**:
```json
{
  "min": 12.3,
  "max": 89.4,
  "avg": 45.2,
  "p5": 34.5,
  "p95": 78.1
}
```

**Implementation**:
- Uses existing audio capture infrastructure
- Sends audio to server's `/api/v1/analyze-audio` (includes RNNoise processing)
- Returns raw statistics for UI to interpret

#### POST /api/calibrate/calculate
**Status**: ✅ Implemented

Calculates recommended threshold from background and speech statistics.

**Request**:
```json
{
  "background_stats": {
    "min": 12.3,
    "max": 89.4,
    "avg": 45.2,
    "p5": 34.5,
    "p95": 78.1
  },
  "speech_stats": {
    "min": 234.5,
    "max": 1823.7,
    "avg": 654.3,
    "p5": 290.2,
    "p95": 1456.8
  }
}
```

**Response**:
```json
{
  "recommended_threshold": 117.15
}
```

**Calculation Logic**: `background_p95 × 1.5`
- Balances conservative (fewer false positives) vs sensitive (fewer false negatives)
- Keeps threshold logic in Go (testable, consistent with CLI wizard)
- UI receives final recommendation (doesn't need to know formula)

#### POST /api/calibrate/save
**Status**: ✅ Implemented

Saves threshold to client config YAML file.

**Request**:
```json
{
  "threshold": 117.15
}
```

**Response**:
```json
{
  "success": true,
  "config_path": "/home/user/.config/richardtate/client.yaml"
}
```

**Implementation**:
- Updates `transcription.vad.energy_threshold` in client config (nested structure)
- Uses shared `config.UpdateVADThreshold()` function (same code as CLI calibration)
- YAML parsing preserves comments and structure
- Returns full path for user confirmation

#### GET /api/calibrate/stream (WebSocket)
**Status**: 📋 Future Enhancement

Real-time energy levels during recording for visual feedback.

**Message Format**:
```json
{
  "timestamp": "2025-11-06T18:21:00Z",
  "energy": 145.3,
  "is_speech": true
}
```

## Benefits

### 1. UI-Driven Calibration
- Web UI can provide visual calibration wizard
- Hammerspoon can offer macOS-native calibration
- Terminal UI remains available
- Mobile apps could calibrate remotely

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
The client should handle calibration because:

1. **Direct Microphone Access**: Client already has audio capture infrastructure
2. **Stateful Process**: Calibration is a multi-step stateful wizard
3. **Audio Capture Reuse**: Can use existing audio capture components
4. **Local Configuration**: Results save to client config file
5. **Server Simplicity**: Server only provides stateless analysis endpoint

### Integration Points
- Reuses `client/internal/audio/capture.go` for recording
- Extends `client/internal/api/server.go` with new endpoints
- Updates `client/internal/config/config.go` with results
- Server's `/api/v1/analyze-audio` endpoint remains unchanged

## Hammerspoon Integration

### Visual Calibration Wizard
**Status**: ✅ Implemented
**Location**: `hammerspoon/calibration.lua` (450 lines)

Complete canvas-based calibration UI for macOS:

**Design Features**:
- 3-step wizard: Background → Speech → Results
- Canvas-based floating window (500x400px)
- Dark theme matching macOS aesthetic
- Real-time progress indicators (updates every 0.5s)
- Visual energy comparison bars
- Click-based interaction (mouse events on canvas)

**User Flow**:
1. Press **Ctrl+Alt+C** to open calibration wizard
2. **Step 1** (Blue theme): "Stay silent" → Records 5s background → Shows stats
3. **Step 2** (Orange theme): "Speak normally" → Records 5s speech → Shows stats
4. **Step 3** (Green theme): Visual bars → Recommended threshold → Save/Cancel buttons

**Why Canvas (Not WebView)**:
- Simpler: Pure Lua drawing (~450 lines vs HTML+CSS+JS)
- Faster: No browser engine overhead
- Native: Matches macOS system look
- Lightweight: Minimal dependencies

**Integration**:
- Hotkey: Ctrl+Alt+C (configurable in init.lua)
- Module-based: `require("calibration")` in init.lua
- Error handling with macOS native notifications
- Cleanup on Hammerspoon reload

**Files**:
- `hammerspoon/calibration.lua` - Complete wizard implementation
- `hammerspoon/init.lua` - Integration and hotkey binding

## Implementation Details

### API Constructor Changes
**Breaking Change**: `api.New()` now requires `*config.Config` parameter

**Rationale**:
- Needed for server URL conversion (WebSocket ws:// prefix)
- Required for audio device configuration in calibration
- Provides access to all client settings

**Migration**:
```go
// Old
apiServer := api.New(logger, webrtcClient, debugLog)

// New
apiServer := api.New(cfg, logger, webrtcClient, debugLog)
```

### Server-Side RNNoise Processing
**Critical**: Calibration endpoint applies RNNoise processing before calculating energy statistics

**Implementation** (server/internal/api/server.go):
- API server stores `rnnoiseModelPath` from config
- API server stores `baseLogger` for creating new components
- `/api/v1/analyze-audio` creates temporary RNNoise processor per request
- Processes audio through RNNoise before calculating energy
- Falls back to raw audio if RNNoise unavailable or errors
- Properly cleans up processor with defer

**Why Temporary Processor**:
- Calibration is infrequent (setup only)
- Simpler lifecycle management (no caching needed)
- No state to manage between requests
- Clean separation from production pipelines

**Result**: Calibration threshold recommendations match what production VAD actually sees

### Stateless Design
**Key Decision**: Recording endpoint is phase-agnostic

**Benefits**:
1. **Client flexibility**: Hammerspoon/CLI decides what each recording means
2. **Simpler API**: Endpoint just captures audio and returns stats
3. **Reusability**: Could record 10 samples and pick best background/speech
4. **UI control**: All wizard logic lives in UI layer (Hammerspoon/CLI)

**Calculation Separation**: Threshold calculation is separate endpoint (`/api/calibrate/calculate`)
- Go code maintains calculation logic (testable, version-controlled)
- UI doesn't need to know formula (just displays recommendation)
- Consistent between CLI wizard and Hammerspoon UI

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
- **Config backup**: Creates backup before modification
- **Error recovery**: Graceful fallback to raw audio if RNNoise fails
- **UI state**: Properly cleanup on wizard close or Hammerspoon reload

## Known Limitations

1. **No real-time streaming**: WebSocket endpoint not yet implemented
2. **Single profile**: Can't save multiple calibration profiles (home, office, etc.)
3. **Fixed recording duration**: Hardcoded to 5 seconds
4. **No validation mode**: Can't test current threshold before saving

## Related Systems
- [VAD Calibration Workflow](../workflows/vad-calibration.md) - CLI implementation details
- [Hammerspoon Integration](../architecture/hammerspoon-integration.md) - Main recording UI
- [Per-Client Pipeline](../architecture/per-client-pipeline.md) - How calibrated settings are used
- [Transcription Gotchas](../gotchas/transcription-gotchas.md#vad-calibration-missing-rnnoise-processing) - RNNoise processing fix