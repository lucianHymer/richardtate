# Parakeet MLX Integration

**Status**: Phase 2 Complete ✅ (Shared Worker Pattern Implemented)
**Last Updated**: 2025-11-17

## Overview
Parakeet MLX integration as alternative ASR engine alongside Whisper. Provides faster transcription on Apple Silicon through MLX acceleration, with automatic platform detection and fallback mock for testing.

**CRITICAL**: Uses **SharedParakeetWorker** pattern - ONE persistent subprocess shared across all pipelines, mirroring the SharedWhisperModel approach.

## Architecture

### Core Design
- **Shared worker pattern**: Single persistent Python worker process (not per-pipeline!)
- **IPC protocol**: JSON messages with Base64-encoded audio over stdin/stdout
- **Platform detection**: macOS uses real Parakeet MLX, Linux uses mock
- **Lifecycle management**: Worker starts at server startup, lives for entire server lifetime
- **Thread-safe**: Mutex-protected communication for concurrent pipeline access

### Component Integration
```
Server Startup → SharedParakeetWorker (persistent subprocess) ←── Multiple Pipelines
                          ↓                                         (via ParakeetTranscriber adapters)
                 parakeet_worker.py (macOS) or parakeet_mock.py (Linux)
```

**Memory Efficiency**:
- **Before (broken)**: N pipelines = N subprocesses = N × model loads (~200MB each)
- **After (fixed)**: N pipelines = 1 subprocess = 1 × model load (~200MB total)
- **Savings**: 10 concurrent clients: 2GB → 200MB (90% reduction!)

## Key Components

### 1. SharedParakeetWorker (`parakeet_shared.go`)
**Location**: `server/internal/transcription/parakeet_shared.go`

**Purpose**: Manages the single persistent Python subprocess shared across all pipelines

**Responsibilities**:
- Launch Python worker subprocess at server startup
- Manage stdin/stdout/stderr communication
- Encode/decode IPC messages (JSON + Base64)
- Monitor subprocess health
- Handle graceful shutdown at server shutdown

**Lifecycle**:
1. **Server startup**: Launch subprocess with configured model path
2. **Startup verification**: Wait for readiness (60s timeout), send test request
3. **Runtime**: Process transcription requests from multiple pipelines concurrently
4. **Server shutdown**: Graceful shutdown with 5s force-kill timeout

**Thread Safety**:
- Mutex protects stdin/stdout/stderr access (subprocess is single-threaded)
- Multiple pipelines can call Transcribe() concurrently (requests are serialized)
- Background goroutine monitors stderr output
- Background goroutine monitors process exit

**Initialization** (main.go):
```go
if engine == "parakeet" {
    sharedParakeetWorker, err = transcription.NewSharedParakeetWorker(transcription.ParakeetConfig{
        ModelPath:  cfg.Transcription.Parakeet.ModelID,
        ScriptPath: cfg.Transcription.Parakeet.ScriptPath,
        PythonPath: cfg.Transcription.Parakeet.PythonPath,
        Logger:     log,
    })
    defer sharedParakeetWorker.Close() // Clean up on server shutdown
}
```

### 2. ParakeetTranscriber (`parakeet_transcriber.go`)
**Location**: `server/internal/transcription/parakeet_transcriber.go`

**Purpose**: Lightweight adapter for SharedParakeetWorker (mirrors WhisperAdapter pattern)

**Responsibilities**:
- Hold reference to shared worker
- Forward Transcribe() calls to shared worker
- No-op Close() (doesn't close shared worker - it lives for entire server lifetime)

**Why Adapter Pattern**:
- Each pipeline gets its own ParakeetTranscriber instance
- All instances share the same SharedParakeetWorker
- Separates pipeline lifecycle from worker lifecycle
- Enables clean shutdown (pipelines close without affecting worker)

**Lightweight Design** (~40 lines vs previous ~300 lines):
```go
type ParakeetTranscriber struct {
    worker *SharedParakeetWorker  // Reference to shared worker
}

func (pt *ParakeetTranscriber) Transcribe(samples []float32) (string, error) {
    return pt.worker.Transcribe(samples)  // Forward to shared worker
}

func (pt *ParakeetTranscriber) Close() error {
    return nil  // No-op - shared worker lives for entire server lifetime
}
```

### 3. Python Worker Script (`parakeet_worker.py`)
**Location**: `scripts/parakeet_worker.py`

**Platform**: macOS only (requires MLX framework)

**Responsibilities**:
- Load Parakeet MLX model on startup
- Listen for JSON requests on stdin
- Decode Base64 audio to float32 samples
- Transcribe audio using Parakeet MLX
- Return JSON responses on stdout
- Log errors to stderr

**Model Loading**:
- Model ID from command-line argument (HuggingFace identifier, not file path)
- Default: `mlx-community/parakeet-tdt-0.6b-v3`
- MLX framework accelerated for Apple Silicon
- **Automatic download**: Models downloaded on first use by parakeet_mlx.from_pretrained()
- **Cache location**: `~/.cache/parakeet-mlx/` (NOT `~/.cache/huggingface/`)
- **No manual download required**: Just specify model ID in config, downloads automatically on first server startup

**Error Handling**:
- Startup errors logged to stderr
- Transcription errors returned as JSON error responses
- Graceful exit on EOF/SIGTERM

### 4. Mock Worker Script (`parakeet_mock.py`)
**Location**: `scripts/parakeet_mock.py`

**Platform**: Linux (or any platform for testing)

**Purpose**: Testing and development without MLX dependencies

**Behavior**:
- Returns mock transcription: "Mock transcription: [N samples]"
- Same IPC protocol as real worker
- Fast response for testing pipeline integration

### 5. Factory Integration (`asr_factory.go`)
**Location**: `server/internal/transcription/asr_factory.go`

**Engine Selection** (updated for shared worker):
```go
switch engine {
case "whisper":
    return NewWhisperAdapter(config.SharedWhisperModel, config.WhisperConfig)
case "parakeet":
    if config.SharedParakeetWorker == nil {
        return nil, fmt.Errorf("SharedParakeetWorker is required for parakeet engine")
    }
    return NewParakeetTranscriber(config.SharedParakeetWorker)  // Pass shared worker
default:
    return nil, fmt.Errorf("unsupported ASR engine: %s", engine)
}
```

### 6. Configuration Structures

**ASRConfig** (updated for shared worker):
```go
type ASRConfig struct {
    Engine               string                 // "whisper" or "parakeet"
    SharedWhisperModel   *SharedWhisperModel    // For Whisper engine (shared across pipelines)
    WhisperConfig        WhisperConfig          // Whisper-specific configuration
    SharedParakeetWorker *SharedParakeetWorker  // For Parakeet engine (shared across pipelines)
}
```

**ParakeetConfig** (unchanged - used for SharedParakeetWorker initialization):
```go
type ParakeetConfig struct {
    ModelPath  string         // Model ID (e.g., "mlx-community/parakeet-tdt-0.6b-v3")
    ScriptPath string         // Path to parakeet_worker.py script
    PythonPath string         // Path to Python interpreter (default: "python3")
    Logger     *logger.Logger // Logger instance
}
```

**ManagerConfig** (updated for shared worker):
```go
type ManagerConfig struct {
    Engine               string                  // ASR engine: "whisper" or "parakeet"
    SharedWhisperModel   *SharedWhisperModel     // Shared Whisper model (for Whisper engine)
    WhisperConfig        WhisperConfig           // Whisper-specific configuration
    SharedParakeetWorker *SharedParakeetWorker   // Shared Parakeet worker (for Parakeet engine)
    RNNoiseModelPath     string
    EnableDebugWAV       bool
}
```

**PipelineConfig** (updated for shared worker):
```go
type PipelineConfig struct {
    Engine               string                 // ASR engine: "whisper" or "parakeet"
    SharedWhisperModel   *SharedWhisperModel    // Shared model (for Whisper engine)
    WhisperConfig        WhisperConfig          // Whisper-specific configuration
    SharedParakeetWorker *SharedParakeetWorker  // Shared worker (for Parakeet engine)
    RNNoiseModelPath     string
    // ... other VAD/chunker settings
}
```

## IPC Protocol

### Request Format
```json
{
  "audio": "base64-encoded-float32-samples",
  "sample_rate": 16000
}
```

**Audio Encoding**:
1. int16 samples converted to float32 (-1.0 to 1.0 range)
2. float32 array serialized to bytes (little-endian)
3. Bytes encoded to Base64 string

### Response Format

**Success**:
```json
{
  "text": "This is the transcribed text."
}
```

**Error**:
```json
{
  "error": "Error message describing what went wrong"
}
```

### Communication Flow
```
Go Process                       Python Process
    |                                  |
    | -- JSON request via stdin -->   |
    |                                  | (decode, transcribe)
    | <-- JSON response via stdout -- |
    |                                  |
```

**stderr**: Used for Python logging/errors (monitored by Go)

## Configuration

### Server Config (config.yaml)
```yaml
transcription:
  engine: "parakeet"  # or "whisper" (default: "whisper")
  model_path: "mlx-community/parakeet-tdt-0.6b-v3"  # Engine-specific (HuggingFace ID for Parakeet)
```

**Engine Selection**:
- `engine: "whisper"` - Uses Whisper with GGML model file path
- `engine: "parakeet"` - Uses Parakeet MLX with HuggingFace model identifier

**Model Path Interpretation**:
- **Whisper**: File path to GGML model (e.g., `/path/to/ggml-large-v3-turbo.bin`)
- **Parakeet**: HuggingFace model identifier (e.g., `mlx-community/parakeet-tdt-0.6b-v3`) - NOT a file path
- **Cache Location**: Parakeet models auto-downloaded to `~/.cache/parakeet-mlx/` on first use, Whisper uses local file path
- **Download**: Parakeet models downloaded automatically on first server startup with `engine: "parakeet"`, no manual download needed

### Manager Configuration
**Location**: `server/internal/webrtc/manager.go`

The WebRTC manager stores engine and model path:
```go
type Manager struct {
    engine    string  // "whisper" or "parakeet"
    modelPath string  // Engine-specific model path/ID
    // ...
}
```

Passed to pipeline creation for each client connection.

## Lifecycle Management

### Startup Sequence
1. Launch Python subprocess: `python3 scripts/parakeet_worker.py <model_path>`
2. Monitor stderr in background goroutine
3. Monitor process exit in background goroutine
4. Wait for subprocess to print "READY" (60s timeout)
5. Send test request: `{"audio": "", "sample_rate": 16000}`
6. Verify response format
7. Ready for transcription requests

**Startup Errors**:
- Model loading failures logged to stderr
- Timeout if "READY" not received in 60s
- Test request failure indicates IPC protocol issue

### Runtime Operation
- Each `Transcribe()` call sends JSON request, reads JSON response
- Mutex ensures only one request in-flight at a time
- Background goroutines continue monitoring stderr and process health

### Shutdown Sequence
1. Close stdin (signals EOF to Python)
2. Wait for process exit (5s timeout)
3. Force kill if still running
4. Join background goroutines
5. Clean up resources

## Platform Detection

### Automatic Selection
**Location**: `server/internal/transcription/parakeet_transcriber.go`

```go
workerScript := "scripts/parakeet_worker.py"
if runtime.GOOS != "darwin" {
    workerScript = "scripts/parakeet_mock.py"
}
```

**Detection Logic**:
- **macOS** (`darwin`): Use real Parakeet MLX worker
- **Linux/other**: Use mock worker for testing

**Why Platform-Specific**:
- MLX framework only available on macOS (Apple Silicon)
- Mock enables testing and development on Linux
- No code changes needed - automatic detection

## Performance Characteristics

### Latency
- **Subprocess startup**: ~2-5 seconds (model loading)
- **Per-transcription overhead**: ~50-100ms (IPC + encoding)
- **Transcription time**: Depends on audio length and model
- **MLX acceleration**: Significantly faster than Whisper on Apple Silicon

### Resource Usage
- **Additional process**: Python subprocess (~200MB RAM)
- **IPC overhead**: Base64 encoding (~33% size increase)
- **Subprocess lifetime**: Lives for entire server session

### Throughput
- **Sequential processing**: Mutex ensures one transcription at a time
- **Multiple clients**: Each pipeline has own ParakeetTranscriber subprocess
- **Scalability**: Linear with client count (one subprocess per pipeline)

## Error Handling

### Startup Failures
- **Model not found**: Error logged to stderr, subprocess exits
- **MLX not available**: Mock automatically used on Linux
- **Timeout**: Returns error after 60s if "READY" not received

### Runtime Failures
- **Subprocess exit**: Detected by background goroutine, future transcriptions fail
- **IPC errors**: JSON decode errors returned to caller
- **Transcription errors**: Returned as error response in JSON

### Recovery
- **No automatic restart**: Failed subprocess stays failed
- **Pipeline recreated**: New client connection creates new subprocess
- **Graceful degradation**: Error propagated up to pipeline, logged

## Testing Strategy

### Mock Worker Benefits
1. **Linux development**: Can develop/test without macOS
2. **CI/CD**: Tests pass on Linux build servers
3. **Fast iteration**: No model loading overhead
4. **Predictable output**: Known mock responses

### Integration Testing
- End-to-end with mock worker (Linux)
- Subprocess lifecycle verification
- IPC protocol correctness
- Error handling scenarios

### Manual Testing
- Real Parakeet MLX on macOS
- Audio quality comparison with Whisper
- Performance benchmarking
- Resource usage monitoring

## Installation

### Automatic via build-mac.sh
**Location**: `scripts/build-mac.sh` (lines 74-208)

**What It Does**:
1. Installs Python dependencies from `scripts/requirements-parakeet.txt`
2. Installs MLX framework if not present
3. Sets up worker scripts (model downloaded automatically on first use)
4. Verifies Python environment

**Usage**:
```bash
./scripts/build-mac.sh
```

### Manual Installation

**Python Dependencies** (REQUIRED):
```bash
# Option 1: Use installation script
./scripts/install-parakeet.sh

# Option 2: Install directly
pip3 install -r scripts/requirements-parakeet.txt
```

**Dependencies** (`scripts/requirements-parakeet.txt`):
- numpy >= 1.24.0
- parakeet-mlx >= 0.1.0

**Model Download** (automatic on first use):
```bash
# Models cached in ~/.cache/parakeet-mlx/
# First server startup with engine: "parakeet" downloads model automatically
# No manual download needed - parakeet_mlx.from_pretrained() handles it
```

**Verify Installation**:
```bash
# Test that worker script can run without import errors
python3 scripts/parakeet_worker.py --help
```

## Design Decisions

### Why Subprocess (Not In-Process)?
1. **Language isolation**: Python for Parakeet MLX, Go for server
2. **Crash isolation**: Python errors don't crash Go server
3. **Resource management**: Easy to limit/monitor Python process
4. **Flexibility**: Can restart subprocess independently

### Why JSON+Base64 IPC?
1. **Simple protocol**: Easy to implement and debug
2. **Language-agnostic**: Works with any language
3. **Text-based**: Can inspect with standard tools
4. **Structured**: JSON provides clear request/response format

### Why Platform Detection?
1. **Development flexibility**: Can work on Linux without MLX
2. **Testing**: Mock enables automated testing
3. **CI/CD**: Build/test on Linux servers
4. **No conditionals**: Automatic based on runtime.GOOS

### Why Shared Worker (Not Per-Pipeline)?
1. **Memory efficiency**: One model load vs N model loads (90% reduction for 10 clients)
2. **Startup speed**: Clients connect instantly (no model loading wait)
3. **Mirrors Whisper pattern**: Consistent architecture across ASR engines
4. **Thread-safe**: Mutex protects subprocess access (requests serialized)
5. **Scalability**: Constant memory regardless of client count

## Known Limitations

1. **Sequential transcription**: Mutex prevents concurrent requests (single subprocess is single-threaded)
2. **Single subprocess**: All pipelines share one worker (failure affects all clients)
3. **No automatic restart**: Failed subprocess requires server restart
4. **macOS-only real implementation**: Linux uses mock only

## Future Enhancements

### Automatic Worker Restart
Automatically restart failed shared worker:
```go
func (w *SharedParakeetWorker) Transcribe(samples []float32) (string, error) {
    if !w.isHealthy() {
        if err := w.restart(); err != nil {
            return "", err
        }
    }
    // ... existing transcription logic
}
```

### Multiple Worker Pool
Multiple shared workers for higher concurrent throughput:
```go
type ParakeetWorkerPool struct {
    workers []*SharedParakeetWorker
    nextIdx int  // Round-robin
}
```

### Streaming Support
Stream partial results during transcription:
```go
type StreamingParakeetWorker struct {
    SharedParakeetWorker
    results chan PartialResult
}
```

### Performance Metrics
Track shared worker performance:
```go
type ParakeetMetrics struct {
    ActivePipelines      int           // Current pipelines using worker
    TotalTranscriptions  int           // Lifetime transcription count
    AverageLatency       time.Duration // Average transcription time
    WorkerRestarts       int           // How many times worker restarted
}
```

## Streaming Integration (November 2024)

### Overview
Successfully integrated Parakeet streaming support for real-time transcription without VAD pauses.

### Key Changes

1. **Type Consolidation**: Kept `SharedParakeetWorker` name (avoided creating duplicate `SharedParakeetWorkerStreaming` type)

2. **Streaming Protocol**: JSON messages with commands:
   - `start_stream`: Initialize new streaming session with client ID
   - `add_audio`: Add audio samples to existing stream
   - `end_stream`: Finalize stream and get remaining text

3. **Client-Based Streaming**: Each ParakeetTranscriber creates unique client ID on init, manages its own streaming session

4. **1-Second Buffering**: Parakeet internally accumulates 16000 samples (1 second at 16kHz) before transcribing, providing natural real-time flow

5. **Pipeline Integration**: Modified `ProcessChunk()` to bypass VAD/chunker for Parakeet engine - sends audio directly to ASR since Parakeet handles its own buffering

### Benefits
- Natural speech flow without VAD pauses
- Real-time feedback every second
- No VAD calibration needed
- Better user experience for continuous speech

### Python Worker
- Uses `parakeet_worker_streaming.py` with `transcribe_stream()` API
- Maintains streaming contexts per client ID
- Handles incremental text tracking (API returns accumulated text, worker sends only new portions)

## Related Documentation
- [ASR Interface Abstraction](asr-interface-abstraction.md) - Interface design and Phase 1
- [Whisper Model Sharing](whisper-model-sharing.md) - Whisper implementation
- [Transcription Pipeline](transcription-pipeline.md) - Pipeline integration
- [Per-Client Pipeline](per-client-pipeline.md) - How pipelines are created

## Files

**Shared Worker Architecture**:
- `server/internal/transcription/parakeet_shared.go` - SharedParakeetWorker with streaming support
- `server/internal/transcription/parakeet_transcriber.go` - Client-based streaming adapter
- `server/cmd/server/main.go` - Worker initialization at startup

**Integration Points**:
- `server/internal/transcription/asr_factory.go` - Factory pattern (accepts SharedParakeetWorker)
- `server/internal/transcription/pipeline.go` - Pipeline config with Parakeet bypass logic
- `server/internal/webrtc/manager.go` - Manager storage (stores SharedParakeetWorker)

**Python Workers**:
- `scripts/parakeet_worker_streaming.py` - Streaming Parakeet MLX worker (macOS)
- `scripts/parakeet_worker.py` - Non-streaming Parakeet MLX worker (legacy)
- `scripts/parakeet_mock.py` - Mock worker for testing (Linux)
- `scripts/build-mac.sh` - Installation automation

**Configuration**:
- `server/internal/config/config.go` - Server configuration
- `server/config.example.yaml` - Configuration documentation
