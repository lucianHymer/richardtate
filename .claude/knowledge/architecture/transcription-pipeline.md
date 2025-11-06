# Transcription Pipeline Architecture

**Status**: RNNoise + VAD Integration Complete ✅
**Last Updated**: 2025-11-06

## Overview
Real-time speech-to-text pipeline with RNNoise noise suppression, VAD-based smart chunking, and Whisper.cpp transcription. Delivers near real-time streaming transcriptions with intelligent silence detection.

## Architecture Flow

```
Raw Audio (200ms chunks, 16kHz)
  → RNNoise (denoise, 16kHz↔48kHz resampling)
  → VAD (detect speech/silence)
  → Smart Chunker (1s silence trigger)
  → Whisper
  → Streaming Results
```

## Core Components

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

### 2. RNNoise Processor (`rnnoise_real.go`)
**Location**: `server/internal/transcription/rnnoise_real.go` (requires `-tags rnnoise`)

**Purpose**: Real-time noise suppression using RNNoise neural network

**Implementation:**
- Processes audio in 10ms frames (160 samples at 16kHz)
- Uses `github.com/xaionaro-go/audio` RNNoise implementation
- Model: `/workspace/project/models/rnnoise/lq.rnnn` (leavened-quisling)
- Automatic buffering for incomplete frames
- Format conversion: int16 ↔ float32

**Sample Rate Conversion:**
- RNNoise requires 48kHz (neural network trained at 48kHz)
- Pipeline uses 16kHz (WebRTC standard)
- Solution: 16kHz → 48kHz (3x upsample) → RNNoise → 16kHz (3x downsample)
- Perfect integer ratio (3x) enables simple linear interpolation
- See `resample.go` for implementation

**Build Requirements:**
- Build tag: `-tags rnnoise`
- System library: librnnoise (build from source, NOT Homebrew package)
- Install: `./scripts/install-rnnoise-lib.sh`
- Without tag: Falls back to pass-through (no denoising)

### 3. Voice Activity Detector (`vad.go`)
**Location**: `server/internal/transcription/vad.go`

**Purpose**: Detects speech vs silence to trigger intelligent chunking

**Implementation:**
- Energy-based VAD (simple, effective)
- Configurable energy threshold (default: 500.0)
- Tracks consecutive speech/silence frames
- 10ms frame analysis (160 samples at 16kHz)
- Signals when silence threshold reached (1 second default)

**VAD Statistics:**
- `SpeechDuration` - Total detected speech time
- `SilenceDuration` - Total silence time
- `TotalDuration` - Overall audio duration

**Key Functions:**
- `NewVAD(config VADConfig)` - Initialize with threshold
- `ProcessFrame(samples []int16)` - Analyze 10ms frame
- `IsSilence()` - Current state query
- `GetStats()` - Duration statistics

### 4. Smart Chunker (`chunker.go`)
**Location**: `server/internal/transcription/chunker.go`

**Purpose**: VAD-driven audio accumulation with intelligent chunking

**Chunking Strategy:**
- Chunks on 1 second of continuous silence (natural speech pauses)
- Min chunk duration: 500ms (avoids tiny chunks)
- Max chunk duration: 30 seconds (safety limit)
- **Critical**: Min speech duration: 1 second (prevents Whisper hallucinations)

**Behavior:**
- Buffers denoised audio from RNNoise
- Monitors VAD for silence detection
- Triggers chunk flush on silence threshold
- Async callback to pipeline (non-blocking)
- Comprehensive stats tracking

**Key Functions:**
- `NewChunker(config ChunkerConfig, callback)` - Initialize
- `AddSamples(samples []int16, vadStats)` - Buffer with VAD data
- `Flush()` - Force immediate chunk
- `GetStats()` - Chunking statistics

### 5. Pipeline Orchestrator (`pipeline.go`)
**Location**: `server/internal/transcription/pipeline.go`

**Purpose**: Coordinates all components into unified streaming pipeline

**Components:**
- Whisper transcriber
- RNNoise processor
- Voice activity detector
- Smart chunker
- Result channel for transcriptions
- Lifecycle management

**Pipeline Flow:**
1. `ProcessChunk()` receives 200ms audio (16kHz int16)
2. RNNoise denoises in 10ms frames
3. VAD analyzes denoised audio
4. Chunker accumulates with VAD stats
5. On silence threshold: chunk sent to Whisper
6. Transcription delivered via `Results()` channel

**Public Interface:**
- `NewPipeline(config PipelineConfig)` - Create pipeline
- `Start()` - Begin processing
- `ProcessChunk(data []int16)` - Handle incoming audio
- `Results()` - Read channel for transcriptions
- `Stop()` - Graceful shutdown with flush
- `Close()` - Release resources

**Debug Features:**
- Optional WAV file export (controlled by config)
- Saves chunks to `/tmp/chunk-*.wav` for analysis

### 6. Resampler (`resample.go`)
**Location**: `server/internal/transcription/resample.go`

**Purpose**: 16kHz ↔ 48kHz conversion for RNNoise compatibility

**Implementation:**
- `Upsample16to48()` - Linear interpolation (160 → 480 samples)
- `Downsample48to16()` - Averaging decimation (480 → 160 samples)
- Float32 variants for RNNoise
- Int16 variants for pipeline integration

**Quality Considerations:**
- Linear interpolation sufficient for 3x ratio
- Could upgrade to sinc interpolation if needed
- Prioritizes shipping over perfect quality

## Key Design Decisions

### RNNoise Integration
**Decision**: Use real RNNoise with sample rate conversion

**Rationale:**
1. Massive quality improvement in noisy environments
2. 16kHz↔48kHz conversion is manageable with 3x ratio
3. Build tag allows graceful fallback without library
4. ~2-3x CPU overhead acceptable for quality gain

### VAD-Based Chunking
**Decision**: Chunk on 1 second of silence, not fixed duration

**Advantages:**
- Natural speech boundaries (sentence/phrase ends)
- Near real-time feedback (1-3s latency)
- Prevents mid-sentence cuts
- Only loses current chunk on crash (not entire session)

### Speech Duration Gating
**Decision**: Require minimum 1 second of actual speech before transcribing

**Rationale:**
- Prevents Whisper hallucinations on noise-only chunks
- Filters out brief noise spikes
- Ensures sufficient content for accurate transcription

### Energy-Based VAD First
**Decision**: Start with simple energy threshold, not WebRTC VAD

**Rationale:**
1. Simple to implement and tune
2. Effective for most use cases
3. Can upgrade to WebRTC VAD if needed
4. Fewer dependencies

## Configuration

### PipelineConfig Structure
```go
type PipelineConfig struct {
    ModelPath       string      // Path to GGML model
    RNNoiseModelPath string     // Path to RNNoise model
    VAD             VADConfig   // VAD parameters
    EnableDebugWAV  bool        // Export chunks as WAV
}

type VADConfig struct {
    Enabled            bool
    EnergyThreshold    float64
    SilenceThresholdMs int
    MinChunkDurationMs int
    MaxChunkDurationMs int
}
```

### Recommended Settings
```go
config := PipelineConfig{
    ModelPath:        "/workspace/project/models/ggml-large-v3-turbo.bin",
    RNNoiseModelPath: "/workspace/project/models/rnnoise/lq.rnnn",
    VAD: VADConfig{
        Enabled:            true,
        EnergyThreshold:    500.0,  // Tune based on mic sensitivity
        SilenceThresholdMs: 1000,   // 1 second silence triggers chunk
        MinChunkDurationMs: 500,    // Avoid tiny chunks
        MaxChunkDurationMs: 30000,  // 30 second safety limit
    },
    EnableDebugWAV: false,  // Set true for debugging
}
```

## Performance Characteristics

### Latency
- **RNNoise overhead**: ~10ms per frame
- **VAD overhead**: Negligible
- **Chunking delay**: 1-3 seconds (silence detection)
- **Transcription time**: Depends on model and hardware
- **Metal acceleration (macOS)**: 40x faster than CPU
- **Total latency**: ~1-3 seconds from speech to text

### CPU Usage
- **RNNoise**: +2-3x overhead (upsampling + processing + downsampling)
- **VAD**: Negligible
- **Whisper**: Depends on model and hardware

### Memory Usage
- **Model loading**: ~1.6GB (Whisper large-v3-turbo)
- **Audio buffer**: ~96KB per 3 seconds at 16kHz
- **RNNoise buffers**: ~1KB per processor
- **Context overhead**: Minimal

### Throughput
- **Input rate**: 200ms chunks at 16kHz (3200 samples/chunk)
- **Processing**: Real-time (can keep up with live audio)
- **Output**: Variable based on speech content

## Thread Safety

All components use mutex protection:
- RNNoise protects frame buffer
- VAD protects state counters
- Chunker protects audio buffer and stats
- Pipeline coordinates goroutine lifecycle
- Results delivered via thread-safe channels

## Error Handling

### Model Loading Failures
- Validates model files exist
- Checks format compatibility
- Returns error on initialization failure

### Processing Failures
- Logs errors but continues processing
- Empty results on transcription failure
- Pipeline remains operational

### Resource Cleanup
- `Close()` releases all contexts
- Proper goroutine termination
- No resource leaks on shutdown

## Build Requirements

### With RNNoise (Recommended)
```bash
# Install RNNoise library
./scripts/install-rnnoise-lib.sh

# Build with RNNoise enabled
./scripts/build-mac.sh  # macOS (auto-detects RNNoise)
# OR manually:
go build -tags rnnoise -o cmd/server/server ./cmd/server
```

### Without RNNoise (Pass-through)
```bash
# Build without noise suppression
go build -o cmd/server/server ./cmd/server
```

### Dependencies
- CGO enabled
- Whisper.cpp library
- RNNoise library (optional, for `-tags rnnoise`)
- Environment variables (see `setup-env.sh`)

## Advantages Over Previous Approaches

### Before (Fixed Duration Buffering):
- Buffered entire recording
- Transcribed only on Stop
- No feedback during speaking
- Could lose everything on crash
- No noise reduction
- Transcribed silence

### Now (VAD-Based Streaming):
- Streams transcriptions during recording
- Chunks on natural speech pauses (1s silence)
- Near real-time feedback (1-3s latency)
- Only loses current chunk on crash
- RNNoise removes background noise
- VAD prevents transcribing silence
- Speech duration gating prevents hallucinations

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
- Timing: ~1-3 seconds after speech pause
- Consumer: Server WebRTC handler

## Testing Strategy

### Unit Tests
- Resampling accuracy (16↔48kHz)
- VAD energy threshold detection
- Chunker buffer behavior
- Configuration validation

### Integration Tests
- End-to-end pipeline flow
- WebRTC → RNNoise → VAD → Chunker → Whisper → Results
- Error recovery scenarios
- Silence detection accuracy

### Performance Tests
- Transcription latency measurements
- Memory usage profiling
- CPU overhead with RNNoise
- Concurrent chunk processing

## Known Limitations

1. **Resampling quality**: Linear interpolation (could upgrade to sinc)
2. **48kHz requirement**: RNNoise neural network constraint
3. **CGO dependency**: Requires system libraries
4. **Build tag**: Must remember `-tags rnnoise` or use build script
5. **Energy-based VAD**: Not as sophisticated as WebRTC VAD

## Future Enhancements

1. **WebRTC VAD**: Replace energy-based with WebRTC's VAD
2. **Sinc resampling**: Higher quality 16↔48kHz conversion
3. **16kHz RNNoise**: Train custom model (major undertaking)
4. **Adaptive thresholds**: Auto-tune VAD based on environment
5. **Quality metrics**: Track transcription confidence

## References

**Implementation Files:**
- `server/internal/transcription/whisper.go` - Whisper integration
- `server/internal/transcription/rnnoise_real.go` - RNNoise with resampling (requires `-tags rnnoise`)
- `server/internal/transcription/rnnoise.go` - Pass-through fallback
- `server/internal/transcription/resample.go` - Sample rate conversion
- `server/internal/transcription/vad.go` - Voice activity detection
- `server/internal/transcription/chunker.go` - Smart chunking
- `server/internal/transcription/pipeline.go` - Pipeline orchestration

**Related Documentation:**
- [Whisper and RNNoise Setup](../dependencies/whisper-and-rnnoise-setup.md)
- [WebRTC Reconnection System](webrtc-reconnection-system.md)
