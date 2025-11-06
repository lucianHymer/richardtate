# Per-Client Pipeline Architecture

**Last Updated**: 2025-11-06

## Overview
Architecture for client-controlled transcription pipelines where each client connection has its own pipeline with custom settings. Enables true multi-user support with per-environment optimization.

## Problem Statement

### Previous Architecture Limitations
- VAD energy threshold was in server config (global)
- Noise suppression was build-time only (-tags rnnoise)
- All clients shared same transcription settings
- Server was stateful with global pipeline
- No multi-user support with different environments

### Requirements
- Each client needs different VAD thresholds based on their microphone/environment
- Multiple clients should work simultaneously
- Server should be stateless (process what it's told)
- Calibration results should automatically apply
- No server restart for config changes

## Solution Architecture

### Client-Controlled Settings
Clients send their transcription settings when starting a recording session:

```go
// In control.start message
type ControlStartData struct {
    VADEnergyThreshold    float64 `json:"vad_energy_threshold"`
    SilenceThresholdMs    int     `json:"silence_threshold_ms"`
    MinChunkDurationMs    int     `json:"min_chunk_duration_ms"`
    MaxChunkDurationMs    int     `json:"max_chunk_duration_ms"`
}
```

### Per-Connection Pipelines
Each WebRTC peer connection maintains its own pipeline:

```go
type PeerConnection struct {
    pc              *webrtc.PeerConnection
    dataChannel     *webrtc.DataChannel
    pipeline        *transcription.Pipeline  // Unique per client
    audioBuffer     []byte
    // ...
}
```

### Pipeline Creation Flow
1. Client sends `control.start` with VAD settings
2. Server creates new pipeline with client's settings
3. Pipeline uses shared Whisper model (loaded once)
4. Each pipeline has its own:
   - Whisper context (using shared model)
   - RNNoise processor instance
   - VAD/Chunker with client's thresholds
   - Result channel for transcriptions

## Implementation Details

### Server Changes
**WebRTC Manager** (`server/internal/webrtc/manager.go`):
- No longer has global pipeline
- Each PeerConnection has pipeline field
- Creates pipeline on control.start
- Closes pipeline on connection close

**API Server** (`server/internal/api/server.go`):
- Parses VAD settings from control.start
- Creates pipeline with client settings
- Assigns pipeline to peer connection

**Main** (`server/cmd/server/main.go`):
- Loads Whisper model once at startup
- Passes model to WebRTC manager
- No global pipeline initialization

### Client Changes
**WebRTC Client** (`client/internal/webrtc/client.go`):
- Reads VAD settings from config
- Sends settings in control.start message
- Settings applied per session

**Config** (`client/internal/config/config.go`):
```yaml
transcription:
  vad_energy_threshold: 184.2
  silence_threshold_ms: 1000
  min_chunk_duration_ms: 500
  max_chunk_duration_ms: 30000
```

**Calibration** (`client/internal/calibrate/calibrate.go`):
- Saves threshold to client config
- No longer updates server config
- Settings used on next recording

### Protocol Changes
**Messages** (`shared/protocol/messages.go`):
- Added `ControlStartData` struct
- control.start message includes VAD settings
- Backwards compatible (empty settings use defaults)

## Benefits

### Multi-User Support
- Each client optimized for their environment
- Different microphones/rooms handled correctly
- No interference between clients
- Clean isolation of transcription state

### Stateless Server
- Server processes based on client instructions
- No global configuration state
- Easy horizontal scaling
- Simplified deployment

### Dynamic Configuration
- Change settings without server restart
- Calibration results immediately applied
- Per-session configuration possible
- A/B testing different settings

### Better Resource Management
- Pipelines created on-demand
- Cleaned up on disconnect
- Shared Whisper model (memory efficient)
- No idle resource consumption

## Testing

### Multi-Client Test Script
Created `test-multi-client.sh` to verify:
```bash
#!/bin/bash
# Start two clients with different VAD thresholds
./client --config config1.yaml &  # threshold: 100
./client --config config2.yaml &  # threshold: 200

# Both clients can record simultaneously
# Each uses their own VAD threshold
# Transcriptions work independently
```

### Verification Points
1. Multiple clients connect successfully
2. Each client's VAD threshold is respected
3. Transcriptions don't interfere
4. Pipelines cleaned up on disconnect
5. Server remains responsive

## Migration Path

### From Global to Per-Client
1. ✅ Add VAD settings to protocol
2. ✅ Modify client to send settings
3. ✅ Create pipeline on control.start
4. ✅ Remove global pipeline from server
5. ✅ Update calibration to save client-side
6. ✅ Test with multiple clients

## Future Enhancements

### Dynamic Setting Updates
- Change VAD threshold mid-session
- Update without stopping recording
- Real-time threshold tuning

### Pipeline Profiles
- Predefined settings for environments
- Quick switch between profiles
- Save/load pipeline configurations

### Advanced Settings
- Language selection per client
- Model selection (speed vs accuracy)
- Custom processing chains

## Related Documentation
- [VAD Calibration Workflow](../workflows/vad-calibration.md) - How thresholds are determined
- [Transcription Pipeline](./transcription-pipeline.md) - Pipeline implementation details
- [Client-Controlled Settings](#) - Original design proposal