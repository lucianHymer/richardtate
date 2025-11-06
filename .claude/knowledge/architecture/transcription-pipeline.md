# Phase 2 Transcription Pipeline Architecture

**Status**: Core MVP Complete ✅
**Last Updated**: 2025-11-06

## Overview
Simplified MVP transcription pipeline using Whisper.cpp for real-time speech-to-text. RNNoise and VAD deferred for future enhancement to accelerate core functionality delivery.

## Architecture Components

### 1. Whisper Transcriber (`whisper.go`)
**Location**: `server/internal/transcription/whisper.go`

**Responsibilities:**
- Loads Whisper GGML model from disk
- Creates processing contexts for transcription
- Transcribes float32 audio samples to text
- Converts PCM int16 to float32 format

**Model Requirements:**
- Path: `/workspace/project/models/ggml-large-v3-turbo.bin`
- Size: ~1.6GB
- Format: GGML binary format

**Key Functions:**
- `NewWhisperTranscriber(modelPath)` - Initialize with model
- `Transcribe(samples []float32)` - Convert audio to text
- `convertPCMToFloat32(pcm []int16)` - Format conversion helper

### 2. Audio Accumulator (`accumulator.go`)
**Location**: `server/internal/transcription/accumulator.go`

**Purpose**: Buffers incoming audio chunks until sufficient duration for transcription

**Configuration:**
- **Minimum duration**: 1000ms (1 second) - configurable
- **Maximum duration**: 3000ms (3 seconds) - configurable
- **Sample rate**: 16kHz mono PCM

**Behavior:**
- Accumulates chunks until min duration reached
- Automatically flushes at max duration (prevents unbounded growth)
- Thread-safe with mutex protection
- Callback-based notification on flush

**Key Functions:**
- `NewAccumulator(minDuration, maxDuration, callback)` - Initialize
- `AddChunk(data []int16)` - Buffer incoming audio
- `Flush()` - Force immediate flush
- `Reset()` - Clear buffer

### 3. Pipeline Orchestrator (`pipeline.go`)
**Location**: `server/internal/transcription/pipeline.go`

**Purpose**: Coordinates transcriber and accumulator into unified pipeline

**Components:**
- Whisper transcriber instance
- Audio accumulator instance
- Result channel for transcriptions
- Lifecycle management (Start/Stop/Close)

**Public Interface:**
- `NewPipeline(config PipelineConfig)` - Create pipeline
- `Start()` - Begin processing
- `ProcessChunk(data []int16)` - Handle incoming audio
- `Results()` - Read channel for transcriptions
- `Stop()` - Graceful shutdown
- `Close()` - Release resources

**Pipeline Flow:**
```
WebRTC Audio → ProcessChunk() → Accumulator → [min duration] → Transcriber → Results Channel
```

## Design Decisions

### Simplified MVP Approach
**Decision**: Skip RNNoise and VAD in initial implementation

**Rationale:**
1. Faster path to end-to-end testing
2. Core Whisper integration is highest priority
3. Noise reduction can be added incrementally
4. RNNoise Go package ready when needed

**Future Enhancement Path:**
- Add RNNoise preprocessing (10ms frames)
- Implement VAD for silence detection
- Optimize buffer sizes based on production metrics

### Buffer Duration Parameters
**Minimum 1 second**: Balance between latency and transcription quality
**Maximum 3 seconds**: Prevents memory growth, ensures responsive output

These values are configurable and can be tuned based on:
- User experience requirements
- Model performance characteristics
- Memory constraints

### Thread Safety
All components use mutex protection:
- Accumulator protects buffer access
- Pipeline coordinates goroutine lifecycle
- Results delivered via thread-safe channels

## Integration Points

### Input
Receives audio from WebRTC client:
- Format: 16kHz mono PCM int16
- Chunk size: 200ms (3200 samples)
- Source: `client/internal/webrtc/client.go`

### Output
Delivers transcriptions via channel:
- Type: `string` (transcribed text)
- Delivery: Asynchronous via `Results()` channel
- Consumer: Server handler (to be implemented)

## Dependencies

### Go Packages
- `github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper` - Official Nov 2025 bindings
- `github.com/xaionaro-go/audio` - RNNoise (added for future use)

### External Libraries
- Whisper.cpp native library (`libwhisper.a`)
- GGML model file (large-v3-turbo)

### Build Requirements
- CGO enabled
- Whisper.cpp includes and libraries (see `setup-env.sh`)
- Environment variables configured

## Configuration

### PipelineConfig Structure
```go
type PipelineConfig struct {
    ModelPath       string  // Path to GGML model
    MinDuration     int     // Milliseconds before transcription
    MaxDuration     int     // Milliseconds forced flush
}
```

### Recommended Settings
```go
config := PipelineConfig{
    ModelPath:   "/workspace/project/models/ggml-large-v3-turbo.bin",
    MinDuration: 1000,  // 1 second
    MaxDuration: 3000,  // 3 seconds
}
```

## Performance Characteristics

### Latency
- **Accumulation delay**: 1-3 seconds (configurable)
- **Transcription time**: Depends on model and hardware
- **Metal acceleration (macOS)**: 40x faster than CPU

### Memory Usage
- **Model loading**: ~1.6GB (large-v3-turbo)
- **Audio buffer**: ~96KB per 3 seconds at 16kHz
- **Context overhead**: Minimal (Whisper.cpp handles internally)

### Throughput
- **Input rate**: 200ms chunks at 16kHz (3200 samples/chunk)
- **Processing**: Batched 1-3 second segments
- **Output**: Variable based on speech content

## Error Handling

### Model Loading Failures
- Validates model file exists
- Checks model format compatibility
- Returns error on initialization failure

### Transcription Failures
- Logs errors but continues processing
- Empty results on transcription failure
- Pipeline remains operational

### Resource Cleanup
- `Close()` releases Whisper contexts
- Proper goroutine termination
- No resource leaks on shutdown

## Testing Strategy

### Unit Tests
- Accumulator buffer behavior
- PCM to float32 conversion
- Configuration validation

### Integration Tests
- End-to-end pipeline flow
- WebRTC → Transcription → Results
- Error recovery scenarios

### Performance Tests
- Transcription latency measurements
- Memory usage profiling
- Concurrent chunk processing

## Next Steps (Phase 3)

1. **Server Integration**
   - Connect pipeline to WebRTC handler
   - Stream results back to client
   - Handle connection lifecycle

2. **Enhancement Options**
   - Add RNNoise preprocessing
   - Implement VAD for silence detection
   - Optimize buffer parameters

3. **Production Readiness**
   - Add metrics and monitoring
   - Performance profiling
   - Load testing

## References

**Implementation Files:**
- `server/internal/transcription/whisper.go` - Whisper integration
- `server/internal/transcription/accumulator.go` - Audio buffering
- `server/internal/transcription/pipeline.go` - Pipeline orchestration

**Related Documentation:**
- [Whisper and RNNoise Setup](../dependencies/whisper-and-rnnoise-setup.md)
- [WebRTC Reconnection System](webrtc-reconnection-system.md)
