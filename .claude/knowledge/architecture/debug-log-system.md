# Debug Log System

**Last Updated**: 2025-11-06 (Session 13)

## Overview
Persistent debug logging system for transcription storage with automatic 8MB rolling rotation. Captures all transcription chunks and completed sessions in JSON format for later recovery and analysis.

## Purpose
- Persistent record of all transcriptions (survives crashes/restarts)
- Recovery of lost text (window closed, app crashed, etc.)
- Historical search through past transcriptions
- Session analysis and debugging
- V2 insertion tracking (planned)

## Core Components

### Package Location
`client/internal/debuglog/`

### Key Features
- **Rolling rotation**: 8MB log file automatically rotates to `.1` backup
- **JSON format**: Machine and human readable
- **Sync writes**: Every entry immediately written to disk (never lose data)
- **Three message types**:
  1. `chunk` - Individual transcription chunks as they arrive
  2. `complete` - Full session text when recording stops
  3. `inserted` - Insertion events (V2 - Obsidian, email, etc.)
- **Home directory expansion**: Supports `~/` in paths
- **Disabled mode**: Empty path = no logging
- **Thread-safe**: Mutex protection for concurrent writes

## Configuration

**Client config** (config.yaml):
```yaml
client:
  debug_log_path: "~/.voice-notes/debug.log"  # Empty = disabled
```

**Constants** (hardcoded in debuglog.go):
- Max file size: 8MB (8 * 1024 * 1024 bytes)
- Rotation: FIFO (2 files max: debug.log + debug.log.1)

## Usage

### Initialization
```go
import "github.com/lucianHymer/streaming-transcription/client/internal/debuglog"

// From main()
debugLog, err := debuglog.New(config.Client.DebugLogPath)
if err != nil {
    log.Fatal("Failed to initialize debug log: %v", err)
}
defer debugLog.Close()
```

### Logging Chunks
```go
// Log individual transcription chunk
debugLog.LogChunk(text, chunkID)
```

### Logging Complete Sessions
```go
// Log full session when recording stops
duration := time.Since(sessionStart)
debugLog.LogComplete(fullSessionText, duration)
```

### Logging Insertions (V2)
```go
// Log when text is inserted into application
debugLog.LogInserted("Obsidian", len(text))
```

## Log Entry Formats

### Chunk Entry
```json
{
  "timestamp": "2025-11-06T16:37:42.123456Z",
  "type": "chunk",
  "text": "This is a transcription chunk.",
  "chunk_id": 1
}
```

### Complete Entry
```json
{
  "timestamp": "2025-11-06T16:38:15.789012Z",
  "type": "complete",
  "full_text": "This is a transcription chunk. This is another chunk.",
  "duration_seconds": 6.3
}
```

### Inserted Entry (V2)
```json
{
  "timestamp": "2025-11-06T16:38:16.123456Z",
  "type": "inserted",
  "location": "Obsidian",
  "length": 87
}
```

## Recovery Usage

### Recent Chunks
```bash
# Last 20 chunks
jq 'select(.type=="chunk")' ~/.voice-notes/debug.log | tail -20

# Last complete session
jq -r 'select(.type=="complete") | .full_text' ~/.voice-notes/debug.log | tail -1
```

### Search for Keywords
```bash
# Find chunks containing keyword
jq -r 'select(.text | contains("keyword"))' ~/.voice-notes/debug.log
```

### Session Statistics
```bash
# Count sessions today
jq 'select(.type=="complete")' ~/.voice-notes/debug.log | grep "$(date +%Y-%m-%d)" | wc -l

# Average session duration
jq -r 'select(.type=="complete") | .duration_seconds' ~/.voice-notes/debug.log | awk '{sum+=$1; count++} END {print sum/count}'
```

## Performance Characteristics

### Write Overhead
- Per-chunk: < 1ms (JSON marshal + write)
- Sync overhead: ~1-5ms (acceptable for 1-3s transcription intervals)
- No impact on transcription latency

### Storage Capacity
- 8MB file: ~400,000 chunks (assuming 20 bytes avg per chunk)
- With rotation: 16MB total = 800,000+ chunks
- Real-world: 500k+ words across both files

### Memory Usage
- Minimal: Only current session accumulates in RAM
- Typical session: 100-200 chunks = ~10KB
- No unbounded growth

## Rotation Behavior

### Trigger
When `debug.log` reaches 8MB:
1. Rename `debug.log` → `debug.log.1` (overwrites old backup)
2. Create new empty `debug.log`
3. Continue writing to new file

### FIFO Strategy
- Only 2 files kept (main + 1 backup)
- Oldest data is lost on second rotation
- Simple and predictable

### Manual Archiving
Users can manually archive `.log.1` files if desired:
```bash
cp ~/.voice-notes/debug.log.1 ~/archives/debug-$(date +%Y%m%d).log
```

## Implementation Details

### Thread Safety
- Mutex protects all file operations
- Safe for concurrent goroutines
- Single writer pattern

### Error Handling
- Initialization errors: Fatal (can't proceed without debug log)
- Write errors: Logged but non-fatal (transcription continues)
- Rotation errors: Logged but non-fatal

### Sync vs Buffer
**Decision**: Use sync writes (no buffering)

**Rationale**:
- Never lose data on crash
- Write frequency is low (1-3 second intervals)
- Sync overhead (~1-5ms) is negligible
- Reliability > performance for debug logs

### Home Directory Expansion
Supports `~/` prefix:
```go
path = strings.Replace(path, "~/", homeDir+"/", 1)
```

### Disabled Mode
Empty path = no-op:
```go
if d.file == nil {
    return // Silent no-op
}
```

## Session State Tracking

**Client maintains**:
- `sessionChunks []string` - Accumulates chunks for complete log
- `sessionStart time.Time` - Tracks session duration
- `sessionRecording bool` - Tracks recording state
- `chunkCounter int` - Monotonic chunk IDs

**Flow**:
1. Start recording → reset state
2. Each chunk → append to sessionChunks + log chunk
3. Stop recording → log complete + reset state

## Testing

**Test suite**: `client/internal/debuglog/debuglog_test.go` (239 lines)

**Coverage**:
- Chunk logging
- Complete session logging
- File rotation at 8MB threshold
- Home directory expansion
- Disabled mode
- Thread safety
- Sync write verification

**Notable test**: Rotation test with 90k entries (~408s runtime)

## Integration Points

### Client Main
```go
// main.go
var globalDebugLog *debuglog.DebugLog

func main() {
    globalDebugLog, err = debuglog.New(cfg.Client.DebugLogPath)
    // ...
}

// Message handler
func handleDataChannelMessage(msg protocol.Message) {
    // On chunk
    globalDebugLog.LogChunk(data.Text, chunkCounter)

    // On stop
    fullText := strings.Join(sessionChunks, " ")
    duration := time.Since(sessionStart)
    globalDebugLog.LogComplete(fullText, duration)
}
```

## Design Decisions

### Why JSON?
- Human readable (can inspect with cat/less)
- Machine parseable (jq, grep, awk)
- Structured data (timestamps, types, fields)
- Easy to extend (add new fields without breaking parsing)

### Why Sync Writes?
- Reliability over performance
- Write frequency is low (not a bottleneck)
- Never lose transcriptions on crash
- Users expect debug logs to be complete

### Why 8MB Rotation?
- Balance between capacity and manageability
- 500k+ words is massive (months of transcriptions)
- Prevents unbounded disk growth
- Simple FIFO rotation (no complex archiving)

### Why Not Database?
- Simpler (no SQLite dependency)
- Portable (just copy .log file)
- Easy recovery (jq/grep/awk)
- No schema migrations
- Works on all platforms

## Known Limitations

1. **No compression**: Could gzip rotated files (future enhancement)
2. **No retention policy**: Old .log.1 is overwritten (manual archiving possible)
3. **Single backup**: Only keeps 1 rotated file (could keep more)
4. **No log level filtering**: Logs everything (by design for debug logs)

## Future Enhancements

1. **Automatic archiving**: Compress and archive .log.1 files
2. **Configurable rotation size**: Make 8MB configurable
3. **Multiple backups**: Keep N rotated files instead of 1
4. **Insertion tracking**: Log V2 insertion events (location, app, etc.)
5. **Session metadata**: Log recording duration, word count, etc.

## Related Files

**Implementation**:
- `client/internal/debuglog/debuglog.go` - Core implementation
- `client/internal/debuglog/debuglog_test.go` - Test suite

**Integration**:
- `client/cmd/client/main.go` - Initialization and usage
- `client/internal/config/config.go` - Configuration

**Related Systems**:
- [Logging System](logging-system.md) - Standard application logging
- [Hammerspoon Integration](hammerspoon-integration.md) - Text insertion (uses debug log data)
