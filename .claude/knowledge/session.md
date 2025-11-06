### [14:11] [architecture] Structured Logging System
**Details**: Implemented standardized logging system with proper log levels and structured fields.

**Features:**
- Log levels: DEBUG, INFO, WARN, ERROR, FATAL (filterable)
- Structured logging with key-value fields via `*WithFields()` methods
- Two output formats: text (human-readable) and JSON (machine-readable for log aggregation)
- Component tagging: Each component gets a contextual logger (e.g., [whisper], [pipeline], [rnnoise], [chunker])
- Thread-safe with mutex protection
- Configurable minimum log level

**Configuration (config.yaml):**
```yaml
server:
  log_level: "info"   # Options: debug, info, warn, error, fatal
  log_format: "text"  # Options: text, json
```

**Usage:**
```go
// Create contextual logger for component
log := logger.With("component-name")

// Simple logging
log.Info("message with %s", arg)
log.Debug("only shown when log_level=debug")

// Structured logging with fields
log.InfoWithFields("Transcription complete", map[string]interface{}{
    "duration": "2.5s",
    "text": "transcribed text",
})
```

**Component Tags:**
- [whisper] - Whisper.cpp transcription
- [pipeline] - Transcription pipeline orchestrator
- [rnnoise] - RNNoise noise suppression
- [chunker] - VAD-based smart chunking
- [webrtc] - WebRTC connection management
- [api] - HTTP API server

**Backwards Compatibility:**
Old `debug: true/false` flag still works:
- `debug: true` → equivalent to `log_level: debug`
- `debug: false` → equivalent to `log_level: info`

**Log Level Behavior:**
- DEBUG: Shows everything (RNNoise frames, Whisper audio stats, VAD processing)
- INFO: Transcription results, pipeline events, server startup
- WARN: Warnings, pass-through mode notices, dropped results
- ERROR: Transcription failures, serious errors
- FATAL: Unrecoverable errors (exits program)

**Implementation:**
- Logger: `server/internal/logger/logger.go`
- All raw `log.Printf()` calls replaced with proper logger
- Module names fixed: `yourusername` → `lucianHymer`
**Files**: server/internal/logger/logger.go, server/internal/config/config.go, server/cmd/server/main.go, server/internal/transcription/whisper.go, server/internal/transcription/pipeline.go, server/internal/transcription/chunker.go, server/internal/transcription/rnnoise.go, server/internal/transcription/rnnoise_real.go
---

### [14:18] [gotcha] Config fields that don't actually work
**Details**: Several config fields were defined but never used by the code:

1. `noise_suppression.enabled` - RNNoise is controlled by build tag `-tags rnnoise`, not config
2. `transcription.translate` - Hardcoded to false in whisper.go:57
3. `transcription.use_gpu` - Never used, GPU is auto-detected by Whisper.cpp
4. `vad.enabled` - VAD is always active, can't be disabled

These have been removed from the config struct. RNNoise being build-time is now clearly documented in config.example.yaml.
**Files**: server/internal/config/config.go, server/config.example.yaml, server/cmd/server/main.go
---

### [14:23] [gotcha] Client config fields that don't work
**Details**: The client config had many defined fields that were never actually used:

1. `server.reconnect_delay_ms` - Hardcoded to 1s in webrtc/client.go:64
2. `server.max_reconnect_delay_ms` - Hardcoded to 30s max in webrtc/client.go:482
3. `server.reconnect_backoff_multiplier` - Hardcoded exponential backoff (2^n) in webrtc/client.go:481
4. `audio.sample_rate` - Hardcoded to 16000 in audio/capture.go:13
5. `audio.channels` - Hardcoded to 1 (mono) in audio/capture.go:14
6. `audio.bits_per_sample` - Hardcoded to 16 in audio/capture.go:17
7. `audio.chunk_duration_ms` - Hardcoded to 200ms in audio/capture.go:15

These values are intentionally hardcoded because they're optimized for speech transcription and shouldn't be changed. Only device_name is kept configurable to allow selecting specific microphones.

All unused fields removed from config struct.
**Files**: client/internal/config/config.go, client/config.example.yaml
---

### [14:37] [gotcha] Whisper hallucination on final chunk
**Details**: Whisper hallucinated "thank you" on the final chunk when recording stopped because Flush() was sending whatever remained in the buffer, even if it was mostly silence or trailing noise.

Solution: Apply same speech duration threshold (1 second minimum) to final flush as we do for regular chunks. Now Flush() checks vadStats.SpeechDuration and only transcribes if >= 1 second of actual speech detected. Otherwise, discards the final chunk with debug log message.

This prevents hallucinations on trailing silence while still allowing legitimate final chunks through.
**Files**: server/internal/transcription/chunker.go
---

### [14:47] [workflow] Client transcription display in terminal
**Details**: Client now displays transcriptions in terminal as they arrive from server. Implementation in handleDataChannelMessage() function:

- MessageTypeTranscriptFinal: Shows "✅ {text}" for completed transcriptions
- MessageTypeTranscriptPartial: Shows "📝 [partial] {text}" for partial results (future use)
- Unmarshals protocol.TranscriptData from message JSON
- Errors go to stderr, transcriptions to stdout
- Simple fmt.Printf() - no UI/webview complexity yet

This gives immediate visual feedback to users while keeping implementation simple. Future could add: accumulation of full session text, timestamps, formatting, etc.
**Files**: client/cmd/client/main.go
---

### [14:59] [architecture] Shared Logging System
**Details**: Complete logging system refactoring to use shared structured logger.

**Location**: shared/logger/logger.go (previously in server/internal/logger/)

**Features**:
- Log levels: Debug, Info, Warn, Error, Fatal
- Output formats: Text (default), JSON configurable
- Structured fields support via InfoWithFields(), DebugWithFields(), etc.
- Component tagging via With("component") for contextual logging
- Level filtering (debug mode on/off)
- Thread-safe with mutex protection

**Usage Pattern**:
```go
// Initialize logger
log := logger.New(debugMode)

// Create component logger
componentLog := log.With("audio")

// Basic logging
componentLog.Info("Starting audio capture")
componentLog.Debug("Processing chunk: seq=%d", seqID)
componentLog.Warn("Buffer full, dropping chunk")
componentLog.Error("Failed to connect: %v", err)

// Structured logging with fields
componentLog.InfoWithFields("Device configured", map[string]interface{}{
    "sample_rate": 16000,
    "channels": 1,
    "format": "S16",
})
```

**Component Tags Used**:
- Server: "api", "webrtc", "Whisper", "Pipeline", "RNNoise", "SmartChunker"
- Client: "webrtc", "api", "audio", "message"

**Migration Complete**:
- Removed server/internal/logger/
- Removed client/internal/logger/
- All server files updated to use shared/logger
- All client files updated to use shared/logger
- All fmt.Printf/println replaced with proper logger calls
- Audio capture now uses structured logging with fields

**Output Format**:
Text: `2025/11/06 04:58:23.123456 [INFO] [component] message | key=value`
JSON: `{"timestamp":"2025/11/06 04:58:23.123456","level":"INFO","component":"audio","message":"text","fields":{"key":"value"}}`

**Build Impact**: None - both client and server build successfully
**Files**: shared/logger/logger.go, client/cmd/client/main.go, client/internal/audio/capture.go, client/internal/webrtc/client.go, client/internal/api/server.go, server/cmd/server/main.go
---

### [15:37] [architecture] VAD Calibration Wizard - Server-Side Design Decision
**Details**: **Decision**: VAD calibration wizard will use SERVER-SIDE energy calculation, not client-side.

**Rationale**:
1. VAD implementation lives in `server/internal/transcription/vad.go`
2. Client should remain lightweight (potentially Arduino-compatible!)
3. Guarantees calibration uses EXACT same energy calculation as production VAD
4. Avoids code duplication and drift between client/server implementations

**Implementation Approach**:
- Add calibration API endpoints to server
- Client captures audio during calibration phases
- Client sends audio to server with calibration flags ("background" / "speech")
- Server runs VAD energy calculation and returns statistics
- Client displays results, calculates recommended threshold, offers to save to config

**Architecture Philosophy**: 
Keep client as thin as possible - just audio capture and streaming. All intelligence (VAD, RNNoise, transcription, calibration analysis) lives server-side. This supports future ultra-lightweight clients (embedded devices, mobile, etc.).

**Trade-off**: 
Requires server to be running during calibration, but this is acceptable since server must be running for normal operation anyway.
**Files**: client/internal/calibrate/, server/internal/api/, server/internal/transcription/vad.go
---

### [15:38] [api] VAD Calibration API - Single Stateless Request
**Details**: **Decision**: VAD calibration will use a SINGLE stateless API request, not two separate requests with server-side state.

**API Design**:
```
POST /api/v1/calibrate
{
  "background_audio": [...],  // 5s of background noise PCM data
  "speech_audio": [...]       // 5s of speech PCM data
}

Response:
{
  "background_stats": {
    "min": 12, "max": 89, "avg": 45, "p95": 78
  },
  "speech_stats": {
    "min": 234, "max": 1823, "avg": 654, "p5": 290
  },
  "recommended_threshold": 150
}
```

**Why Single Request**:
1. Server remains stateless (no session management)
2. Simpler API design
3. Client orchestrates the UX (progress bars, timing)
4. Server just does pure calculation
5. More RESTful/functional approach

**Client Workflow**:
1. Show "Recording background..." with progress bar (5s)
2. Show "Recording speech..." with progress bar (5s)
3. Send BOTH samples to server in one request
4. Display results and offer to save

**Trade-off**: Client holds both audio samples in memory (~10 seconds = ~320KB at 16kHz), but this is negligible for modern systems.
**Files**: server/internal/api/, client/internal/calibrate/
---

### [15:45] [workflow] VAD Calibration Wizard - Complete Implementation
**Details**: **Implementation Complete**: VAD calibration wizard using server-side energy calculation.

**How It Works**:
1. Client captures 5 seconds of background noise
2. Client captures 5 seconds of speech
3. Client sends both samples to server `/api/v1/analyze-audio` endpoint (called twice)
4. Server calculates energy statistics using same VAD algorithm as production
5. Client calculates recommended threshold: `(background_p95 + speech_p5) / 2`
6. Client displays visual comparison and offers to save

**Usage**:
```bash
./client --calibrate                    # Interactive mode
./client --calibrate --yes              # Auto-save mode
./client --calibrate --config=path.yaml # Custom config
```

**Server API**:
- Endpoint: `POST /api/v1/analyze-audio`
- Request: `{"audio": [byte array of PCM int16]}`
- Response: `{"min": float, "max": float, "avg": float, "p5": float, "p95": float, "sample_count": int}`
- Uses 10ms frames (160 samples at 16kHz) matching VAD implementation
- Calculates RMS energy per frame, returns statistics

**Client Components**:
- `client/internal/calibrate/calibrate.go` - Main wizard logic with terminal UI
- Flag handling in `client/cmd/client/main.go`
- Uses existing audio capture infrastructure
- HTTP client for server API calls

**Features**:
- Progress bars during recording
- Visual bar chart comparison
- Interactive save confirmation
- Auto-save mode with `--yes` flag
- Reuses exact same energy calculation as production VAD

**TODO (deferred)**:
- Automatic config file update (currently shows manual instructions)
- Could be implemented with YAML parser in future

**Architecture Win**:
Keeps client lightweight by delegating all energy calculation to server. Client just captures audio and displays results.
**Files**: server/internal/api/server.go, client/internal/calibrate/calibrate.go, client/cmd/client/main.go
---

