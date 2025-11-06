# Logging System Architecture

**Last Updated**: 2025-11-06 (Session 13 - Config-driven log levels)

## Overview
Unified structured logging system shared between client and server, built on Go's standard library `log` package with contextual tagging and structured fields support.

## Core Components

### 1. Logger (`logger.go`)
**Location**: `shared/logger/logger.go` (formerly `server/internal/logger/logger.go`)

**Structure**:
- Based on Go standard library `log` package
- No external dependencies (no logrus, zap, or slog)
- Supports both text and JSON output formats
- Thread-safe with mutex protection

**Log Levels**:
- `Debug` - Debug messages (only when debug mode enabled)
- `Info` - Information messages
- `Warn` - Warning messages
- `Error` - Error messages
- `Fatal` - Fatal errors (causes program exit)

**Methods**:
```go
// Basic logging
Info(format string, args ...interface{})
Error(format string, args ...interface{})
Debug(format string, args ...interface{})
Warn(format string, args ...interface{})
Fatal(format string, args ...interface{})

// Structured logging with fields
InfoWithFields(message string, fields map[string]interface{})
ErrorWithFields(message string, fields map[string]interface{})
DebugWithFields(message string, fields map[string]interface{})
WarnWithFields(message string, fields map[string]interface{})
FatalWithFields(message string, fields map[string]interface{})
```

**Configuration**:
```go
type Config struct {
    Level      LogLevel      // Minimum log level to output
    Format     OutputFormat  // Text or JSON
    Output     io.Writer     // Output destination (default: os.Stdout)
    Debug      bool          // Convenience flag to set level to Debug
}
```

**Behavior**:
- Each method prepends level tag: `[INFO]`, `[ERROR]`, `[DEBUG]`, `[WARN]`, `[FATAL]`
- Debug messages only log if debug level enabled
- Flags: `log.LstdFlags | log.Lmicroseconds` (timestamp with microseconds)
- Output: `os.Stdout` by default (configurable)
- Supports structured fields for better parsing

### 2. ContextLogger
**Purpose**: Contextual wrapper for component-tagged logging

**Usage**:
- Created via `logger.With("component")`
- Adds component tag to each log message
- Supports same methods as Logger
- Can chain fields with `WithFields()`

**Example**:
```go
log := logger.New(true)
audioLog := log.With("audio")
audioLog.Info("Starting audio capture")
audioLog.InfoWithFields("Device configured", map[string]interface{}{
    "sample_rate": 16000,
    "channels": 1,
})
```

## Usage Patterns

### Server Components
All server components use the shared logger:
- `server/cmd/server/main.go` - Main logger initialization
- `server/internal/api/server.go` - Uses `logger.With("api")`
- `server/internal/webrtc/manager.go` - Uses `logger.With("webrtc")`
- `server/internal/transcription/whisper.go` - Manual `[Whisper]` tags (legacy style)
- `server/internal/transcription/pipeline.go` - Manual `[Pipeline]` tags (legacy style)
- `server/internal/transcription/rnnoise_real.go` - Manual `[RNNoise]` tags (legacy style)
- `server/internal/transcription/chunker.go` - Manual `[SmartChunker]` tags (legacy style)

### Client Components
All client components use the shared logger with structured logging:
- `client/cmd/client/main.go` - Main logger initialization, global logger for message handler
- `client/internal/webrtc/client.go` - Uses `logger.With("webrtc")`
- `client/internal/api/server.go` - Uses `logger.With("api")`
- `client/internal/audio/capture.go` - Uses `logger.With("audio")` with structured fields

**Client Best Practice** (audio/capture.go):
```go
// Structured logging with fields
c.logger.InfoWithFields("🔍 Actual Device Configuration", map[string]interface{}{
    "sample_rate": c.device.SampleRate(),
    "format":      c.device.CaptureFormat(),
    "channels":    c.device.CaptureChannels(),
})

c.logger.DebugWithFields("🎤 Audio level detected", map[string]interface{}{
    "rms_squared": rms,
    "min_sample":  minSample,
    "max_sample":  maxSample,
    "frames":      framecount,
    "bytes":       len(pSample),
})
```

## Tag Reference

Current tags in use across the codebase:

| Tag | Component | Location | Style |
|-----|-----------|----------|-------|
| `[INFO]` | General information | All components | Automatic |
| `[ERROR]` | Errors | All components | Automatic |
| `[DEBUG]` | Debug output (debug mode) | All components | Automatic |
| `[WARN]` | Warnings | All components | Automatic |
| `[FATAL]` | Fatal errors | All components | Automatic |
| `[api]` | API server | server/internal/api, client/internal/api | ContextLogger |
| `[webrtc]` | WebRTC manager/client | server/internal/webrtc, client/internal/webrtc | ContextLogger |
| `[audio]` | Audio capture | client/internal/audio | ContextLogger |
| `[message]` | Message handler | client/cmd/client | ContextLogger |
| `[Whisper]` | Whisper transcriber | server/internal/transcription | Manual (legacy) |
| `[Pipeline]` | Transcription pipeline | server/internal/transcription | Manual (legacy) |
| `[RNNoise]` | RNNoise processor | server/internal/transcription | Manual (legacy) |
| `[SmartChunker]` | Smart chunker | server/internal/transcription | Manual (legacy) |

## Output Formats

### Text Format (Default)
```
2025/11/06 04:58:23.123456 [INFO] [api] Server started
2025/11/06 04:58:23.456789 [DEBUG] [webrtc] Connection established
2025/11/06 04:58:24.123456 [INFO] [audio] Device configured | sample_rate=16000 channels=1
```

### JSON Format
```json
{"timestamp":"2025/11/06 04:58:23.123456","level":"INFO","component":"api","message":"Server started"}
{"timestamp":"2025/11/06 04:58:24.123456","level":"INFO","component":"audio","message":"Device configured","fields":{"sample_rate":16000,"channels":1}}
```

## Configuration Examples

### Basic Usage (Server)
```go
log := logger.New(debug)
log.Info("Server starting...")
apiLog := log.With("api")
apiLog.Info("Listening on %s", bindAddr)
```

### Advanced Usage (Client with Fields)
```go
log := logger.New(cfg.Client.Debug)
audioLog := log.With("audio")

// Simple logging
audioLog.Info("Starting audio capture")

// Structured logging
audioLog.InfoWithFields("Device selected", map[string]interface{}{
    "device_name": deviceName,
    "sample_rate": 16000,
    "channels": 1,
})
```

### JSON Output (Production)
```go
log := logger.NewWithConfig(logger.Config{
    Level:  logger.LevelInfo,
    Format: logger.FormatJSON,
    Output: os.Stdout,
})
```

## Configuration

**Server** (config.yaml):
```yaml
server:
  log_level: "info"   # Options: debug, info, warn, error, fatal
  log_format: "text"  # Options: text, json
```

**Backwards Compatibility**: Old `debug: true/false` flag still works:
- `debug: true` → equivalent to `log_level: debug`
- `debug: false` → equivalent to `log_level: info`

**Log Level Behavior**:
- DEBUG: Shows everything (RNNoise frames, Whisper audio stats, VAD processing)
- INFO: Transcription results, pipeline events, server startup
- WARN: Warnings, pass-through mode notices, dropped results
- ERROR: Transcription failures, serious errors
- FATAL: Unrecoverable errors (exits program)

## Migration History

**Session 13 (2025-11-06)**: Standardized logging with config-driven levels
- Added log_level and log_format configuration to config.yaml
- Replaced all raw `log.Printf()` calls with proper logger
- Added structured logging with `*WithFields()` methods
- Component tagging standardized across server
- Fixed module names: `yourusername` → `lucianHymer`

**Session 11 (2025-11-06)**: Complete logging unification
- Moved `server/internal/logger/` → `shared/logger/`
- Removed `client/internal/logger/` (old simple logger)
- Updated all imports across client and server
- Replaced all `fmt.Printf`/`println` in client with proper logger calls
- Added structured logging support throughout client
- All components now use shared logger

**Pre-Session 11**: Mixed logging patterns
- Server had structured logger
- Client had simple logger without levels
- `fmt.Printf` scattered throughout client code

## Design Rationale

**Why Shared Logger**:
1. Consistent logging format across client and server
2. Easy to aggregate and parse logs
3. Single source of truth for logging behavior
4. Shared location in `shared/` package makes sense

**Why Custom Logger (Not External Library)**:
1. Minimal dependencies
2. Simple level-based filtering
3. Easy context tagging via `With()`
4. Built on standard library
5. Sufficient for current needs

**Why Structured Fields**:
- Better machine parsing
- Easy to switch to JSON output
- Preserves data types
- Production-ready format

## Known Limitations

1. **Transcription package still uses manual tags**: The transcription components (`whisper.go`, `pipeline.go`, etc.) use `log.Printf()` with manual `[Tag]` prefixes instead of ContextLogger
2. **No log rotation built-in**: Would need external solution for log file rotation
3. **No async logging**: All logging is synchronous (acceptable for current scale)

## Future Considerations

**Potential Improvements**:
1. Migrate transcription package to use ContextLogger instead of manual tags
2. Add log rotation support for file output
3. Add metrics/observability hooks
4. Consider async logging for high-throughput scenarios
5. Add sampling for noisy debug logs

## Related Files

**Core Implementation**:
- `shared/logger/logger.go` - Logger implementation

**Server Usage**:
- `server/cmd/server/main.go` - Logger initialization
- `server/internal/api/server.go` - ContextLogger example
- `server/internal/webrtc/manager.go` - ContextLogger example
- `server/internal/transcription/*.go` - Manual tag examples (legacy)

**Client Usage**:
- `client/cmd/client/main.go` - Logger initialization, global logger setup
- `client/internal/webrtc/client.go` - ContextLogger example
- `client/internal/api/server.go` - ContextLogger example
- `client/internal/audio/capture.go` - Structured logging best practices
