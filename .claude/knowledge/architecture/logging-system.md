# Logging System Architecture

**Last Updated**: 2025-11-06

## Overview
Custom logging system built on Go's standard library `log` package with contextual tagging support.

## Core Components

### 1. Logger (`logger.go`)
**Location**: `server/internal/logger/logger.go`

**Structure**:
- Based on Go standard library `log` package
- No external dependencies (no logrus, zap, or slog)
- Debug mode control via flag

**Methods**:
- `Info()` - Information messages
- `Error()` - Error messages
- `Debug()` - Debug messages (only when debug=true)
- `Warn()` - Warning messages
- `Fatal()` - Fatal errors (causes program exit)

**Behavior**:
- Each method prepends level tag: `[INFO]`, `[ERROR]`, `[DEBUG]`, `[WARN]`, `[FATAL]`
- Debug messages only log if debug mode enabled
- Flags: `log.LstdFlags | log.Lmicroseconds` (timestamp with microseconds)
- Output: `os.Stdout` (console output)
- Format: `[TIMESTAMP LEVEL] [TAG] message`

### 2. ContextLogger
**Purpose**: Contextual wrapper for tagged logging

**Usage**:
- Created via `logger.With("prefix")`
- Adds prefix to each log message
- Used for component-specific logging

**Examples**:
- API server: `log.With("api")`
- WebRTC manager: `log.With("webrtc")`

## Current Logging Patterns

### Structured Approach (High-level components)
**Files using Logger properly**:
- `server/internal/api/server.go` - Uses `logger.With("api")`
- `server/internal/webrtc/manager.go` - Uses `logger.With("webrtc")`
- `server/cmd/server/main.go` - Uses direct Logger methods

### Direct Printf Approach (Low-level components)
**Files bypassing Logger interface**:
- `server/internal/transcription/whisper.go` - Uses `log.Printf()` (5 statements)
- `server/internal/transcription/pipeline.go` - Uses `log.Printf()` (6 statements)
- `server/internal/transcription/rnnoise_real.go` - Uses `fmt.Printf()` (2 statements)
- `server/internal/transcription/chunker.go` - Uses `log.Printf()` (1 statement)

**Manual Tags Used**:
- `[Whisper]` - Whisper transcriber logs
- `[Pipeline]` - Transcription pipeline logs
- `[RNNoise]` - RNNoise processor logs
- `[SmartChunker]` - Smart chunker logs

## Inconsistency Note

The codebase currently has two distinct logging patterns:

1. **High-level components**: Use structured `Logger` with `ContextLogger` tags
2. **Transcription package**: Uses raw `log.Printf()` and `fmt.Printf()` with manual tags

This inconsistency exists because:
- Transcription code bypasses the Logger interface
- Tags are manually added in format strings
- Some use `log.Printf()`, others use `fmt.Printf()`

## Tag Reference

Current tags in use across the codebase:

| Tag | Component | Location |
|-----|-----------|----------|
| `[INFO]` | General information | All components |
| `[ERROR]` | Errors | All components |
| `[DEBUG]` | Debug output (debug mode) | All components |
| `[WARN]` | Warnings | All components |
| `[FATAL]` | Fatal errors | All components |
| `[api]` | API server | server/internal/api |
| `[webrtc]` | WebRTC manager | server/internal/webrtc |
| `[Whisper]` | Whisper transcriber | server/internal/transcription |
| `[Pipeline]` | Transcription pipeline | server/internal/transcription |
| `[RNNoise]` | RNNoise processor | server/internal/transcription |
| `[SmartChunker]` | Smart chunker | server/internal/transcription |

## Configuration

**Debug Mode**:
- Controlled by command-line flag
- When enabled: `Debug()` messages appear
- When disabled: `Debug()` messages suppressed

**Output Format**:
```
2025-11-06 04:58:23.123456 [INFO] [api] Server started
2025-11-06 04:58:23.456789 [DEBUG] [webrtc] Connection established
2025-11-06 04:58:23.789012 [Pipeline] Processing chunk
```

## Design Rationale

**Why Custom Logger**:
1. Minimal dependencies
2. Simple level-based filtering
3. Easy context tagging via `With()`
4. Built on standard library

**Why No External Dependencies**:
- Reduces dependency bloat
- Standard library sufficient for current needs
- Can upgrade later if needed

## Future Considerations

**Potential Improvements**:
1. Standardize transcription package logging to use Logger
2. Replace `fmt.Printf()` with `log.Printf()` or Logger methods
3. Consider structured logging (JSON output) for production
4. Add log levels beyond debug/non-debug
5. Add log file output option

## Related Files

- `server/internal/logger/logger.go` - Logger implementation
- `server/cmd/server/main.go` - Logger initialization
- `server/internal/api/server.go` - Structured logging example
- `server/internal/webrtc/manager.go` - Structured logging example
- `server/internal/transcription/*.go` - Direct Printf logging examples
