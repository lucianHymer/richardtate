# Parakeet MLX Integration

**Status**: Phase 2 Complete ✅
**Last Updated**: 2025-11-17

## Overview
Parakeet MLX integration as alternative ASR engine alongside Whisper. Provides faster transcription on Apple Silicon through MLX acceleration, with automatic platform detection and fallback mock for testing.

## Architecture

### Core Design
- **Subprocess model**: Python worker runs as separate process
- **IPC protocol**: JSON messages with Base64-encoded audio over stdin/stdout
- **Platform detection**: macOS uses real Parakeet MLX, Linux uses mock
- **Lifecycle management**: Automatic startup, health monitoring, graceful shutdown
- **Thread-safe**: Mutex-protected communication

### Component Integration
```
Pipeline → ASRTranscriber Interface → ParakeetTranscriber → Python Subprocess
                                                              ↓
                                         parakeet_worker.py (macOS) or parakeet_mock.py (Linux)
```

## Key Components

### 1. ParakeetTranscriber (`parakeet_transcriber.go`)
**Location**: `server/internal/transcription/parakeet_transcriber.go`

**Responsibilities**:
- Launch Python worker subprocess
- Manage stdin/stdout/stderr communication
- Encode/decode IPC messages (JSON + Base64)
- Monitor subprocess health
- Handle graceful shutdown

**Lifecycle**:
1. Launch subprocess with configured model path
2. Wait for startup (60s timeout)
3. Send test request to verify readiness
4. Monitor stderr output in background
5. Process transcription requests
6. Graceful shutdown on Close() (5s force-kill timeout)

**Threading Model**:
- Mutex protects stdin/stdout/stderr access
- Background goroutine monitors stderr output
- Background goroutine monitors process exit
- All concurrent operations properly synchronized

### 2. Python Worker Script (`parakeet_worker.py`)
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

### 3. Mock Worker Script (`parakeet_mock.py`)
**Location**: `scripts/parakeet_mock.py`

**Platform**: Linux (or any platform for testing)

**Purpose**: Testing and development without MLX dependencies

**Behavior**:
- Returns mock transcription: "Mock transcription: [N samples]"
- Same IPC protocol as real worker
- Fast response for testing pipeline integration

### 4. Factory Integration (`asr_factory.go`)
**Location**: `server/internal/transcription/asr_factory.go`

**Engine Selection**:
```go
switch engine {
case "whisper":
    return &WhisperAdapter{...}, nil
case "parakeet":
    return NewParakeetTranscriber(cfg.ParakeetConfig)
default:
    return nil, fmt.Errorf("unknown ASR engine: %s", engine)
}
```

### 5. Configuration Structures

**ASRConfig** (extended):
```go
type ASRConfig struct {
    Engine             string              // "whisper" or "parakeet"
    SharedWhisperModel whisper.Model       // For whisper engine
    WhisperConfig      WhisperConfig       // Whisper-specific settings
    ParakeetConfig     ParakeetConfig      // Parakeet-specific settings
}
```

**ParakeetConfig** (new):
```go
type ParakeetConfig struct {
    ModelPath string  // Model identifier (e.g., "mlx-community/parakeet-tdt-0.6b-v3")
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

### Why One Subprocess Per Pipeline?
1. **Isolation**: Each client's transcription independent
2. **Simplicity**: No shared state or coordination needed
3. **Scalability**: Linear scaling with client count
4. **Resource control**: Clear per-client resource usage

## Known Limitations

1. **Sequential transcription**: Mutex prevents concurrent requests to same subprocess
2. **No subprocess pooling**: Each pipeline gets dedicated subprocess
3. **No automatic restart**: Failed subprocess stays failed
4. **macOS-only real implementation**: Linux uses mock only

## Future Enhancements

### Subprocess Pooling
Share subprocesses across pipelines:
```go
type ParakeetPool struct {
    workers []*ParakeetTranscriber
    queue   chan transcriptionRequest
}
```

### Automatic Restart
Restart failed subprocess automatically:
```go
if !p.process.Running() {
    p.restart()
}
```

### Streaming Support
Stream partial results during transcription:
```go
type StreamingParakeetTranscriber struct {
    ParakeetTranscriber
    results chan PartialResult
}
```

### Performance Metrics
Track transcription performance:
```go
type ParakeetMetrics struct {
    TranscriptionCount    int
    AverageLatency       time.Duration
    SubprocessRestarts   int
}
```

## Related Documentation
- [ASR Interface Abstraction](asr-interface-abstraction.md) - Interface design and Phase 1
- [Whisper Model Sharing](whisper-model-sharing.md) - Whisper implementation
- [Transcription Pipeline](transcription-pipeline.md) - Pipeline integration
- [Per-Client Pipeline](per-client-pipeline.md) - How pipelines are created

## Files
- `server/internal/transcription/parakeet_transcriber.go` - Go implementation
- `server/internal/transcription/asr_factory.go` - Factory pattern
- `server/internal/transcription/pipeline.go` - Pipeline integration
- `server/internal/webrtc/manager.go` - Engine storage
- `server/internal/config/config.go` - Configuration
- `server/config.example.yaml` - Config documentation
- `server/cmd/server/main.go` - Wiring
- `scripts/parakeet_worker.py` - Real worker (macOS)
- `scripts/parakeet_mock.py` - Mock worker (Linux)
- `scripts/build-mac.sh` - Installation automation
