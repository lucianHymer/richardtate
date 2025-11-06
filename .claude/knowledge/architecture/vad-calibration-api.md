# VAD Calibration API Architecture

**Last Updated**: 2025-11-06

## Overview
Architecture for transitioning VAD calibration from a separate client launch mode to API endpoints, enabling UI-driven calibration and better user experience.

## Current Limitation
The calibration is currently a separate client launch mode (--calibrate flag) which requires:
- Restarting the client to run calibration
- Terminal-only interface
- No real-time feedback
- Static wizard-like flow

## Proposed API Architecture

### Client-Side Endpoints
The calibration should be exposed as API endpoints on the client daemon:

#### POST /api/calibrate/record
Records and analyzes audio for background or speech phase.

**Request**:
```json
{
  "phase": "background" | "speech",
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

#### POST /api/calibrate/save
Saves recommended threshold to config file.

**Request**:
```json
{
  "threshold": 184.2
}
```

**Response**:
```json
{
  "success": true,
  "config_path": "/home/user/.voice-notes/config.yaml"
}
```

#### GET /api/calibrate/stream (WebSocket - Optional)
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

## Implementation Path

### Phase 1: Core API
1. Move calibration logic to API handler
2. Create `/api/calibrate/record` endpoint
3. Create `/api/calibrate/save` endpoint
4. Keep existing CLI interface as wrapper

### Phase 2: Real-Time Feedback
1. Add WebSocket endpoint for energy streaming
2. Implement buffered energy calculation
3. Add live speech detection feedback

### Phase 3: UI Integration
1. Web UI calibration wizard
2. Hammerspoon calibration dialog
3. Settings persistence and profiles

## Related Systems
- [VAD Calibration Workflow](../workflows/vad-calibration.md) - Current CLI implementation
- [Client API Server](../architecture/client-api-server.md) - API infrastructure
- [Per-Client Pipeline](../architecture/per-client-pipeline.md) - How calibrated settings are used