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

