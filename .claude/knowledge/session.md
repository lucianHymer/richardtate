### [02:09] [architecture] RNNoise + VAD Integration for Smart Chunking
**Details**: ## Implementation Complete: RNNoise + VAD-Based Smart Chunking

**Status**: Code complete, awaiting build testing on Mac hardware

### New Pipeline Flow
Raw Audio (200ms chunks) → RNNoise (denoise) → VAD (detect speech) → Smart Chunker (1s silence trigger) → Whisper → Streaming Results

### Components Created

**1. RNNoise Processor** (`server/internal/transcription/rnnoise.go`)
- Processes audio in 10ms frames (160 samples at 16kHz)
- Uses `github.com/xaionaro-go/audio` RNNoise implementation
- Model: `/workspace/project/models/rnnoise/lq.rnnn`
- Buffers incomplete frames automatically
- Converts int16 ↔ float32 for RNNoise compatibility

**2. Voice Activity Detector** (`server/internal/transcription/vad.go`)
- Simple energy-based VAD (upgradeable to WebRTC VAD later)
- Configurable energy threshold (default: 500.0)
- Tracks consecutive speech/silence frames
- 10ms frame analysis (160 samples at 16kHz)
- Signals when silence threshold reached (1 second default)

**3. Smart Chunker** (`server/internal/transcription/chunker.go`)
- VAD-driven audio accumulation
- Chunks on 1 second of continuous silence (configurable)
- Min chunk duration: 500ms (avoids tiny chunks)
- Max chunk duration: 30 seconds (safety limit)
- Async callback to pipeline when chunk ready
- Comprehensive stats tracking

**4. Updated Pipeline** (`server/internal/transcription/pipeline.go`)
- Completely rewritten for streaming architecture
- Transcribes chunks as they become ready (not whole-session)
- Debug WAV file export (optional, controlled by config)
- Proper flush handling on Stop()

### Configuration Updates

**Added to `config.go`:**
```go
VAD struct {
    Enabled            bool
    EnergyThreshold    float64
    SilenceThresholdMs int
    MinChunkDurationMs int
    MaxChunkDurationMs int
}
```

**Added to `config.example.yaml`:**
- RNNoise model path
- VAD parameters (all configurable)
- Debug WAV export toggle

**Server initialization** (`cmd/server/main.go`):
- Reads VAD config from YAML
- Passes to pipeline initialization
- Enables debug WAV in debug mode

### Key Design Decisions

1. **Energy-based VAD first**: Start simple, upgrade to WebRTC VAD if needed
2. **1 second silence trigger**: Good balance for natural speech chunking
3. **Async chunk callback**: Prevents blocking audio ingestion during transcription
4. **Safety limits**: Min/max duration prevents edge cases
5. **Debug WAV export**: Controlled by server debug flag, saves to `/tmp/chunk-*.wav`

### Advantages Over Whole-Session Approach

**Before** (Session 7):
- Buffered entire recording
- Transcribed only on Stop
- No feedback during speaking
- Could lose everything on crash

**Now**:
- Streams transcriptions during recording
- Chunks on natural speech pauses (1s silence)
- Near real-time feedback (1-3s latency)
- Only loses current chunk on crash
- RNNoise removes background noise
- VAD prevents transcribing silence

### Critical Implementation Notes

1. **RNNoise frame size**: 10ms = 160 samples at 16kHz (hardcoded in library)
2. **VAD must see denoised audio**: VAD runs AFTER RNNoise in pipeline
3. **Chunk callback is async**: Uses goroutine to avoid blocking
4. **Buffer management**: All components handle incomplete frames properly
5. **Thread safety**: Mutexes protect all shared state

### Testing Requirements

**Next steps:**
1. Build on Mac with `./scripts/build-mac.sh`
2. Copy `config.example.yaml` to `config.yaml`
3. Set model paths in config
4. Test with real microphone
5. Tune VAD energy threshold based on mic sensitivity
6. Verify chunks trigger on silence, not mid-sentence

**Expected behavior:**
- Speak normally → transcriptions appear ~1-2 seconds after pausing
- Background noise filtered out by RNNoise
- No transcription during long silence
- Clean chunk boundaries at sentence/phrase ends

### Files Modified/Created

**Created:**
- `server/internal/transcription/rnnoise.go` (155 lines)
- `server/internal/transcription/vad.go` (121 lines)
- `server/internal/transcription/chunker.go` (166 lines)

**Modified:**
- `server/internal/transcription/pipeline.go` (completely rewritten, 314 lines)
- `server/internal/config/config.go` (added VAD config)
- `server/config.example.yaml` (added RNNoise + VAD sections)
- `server/cmd/server/main.go` (wire up new config)

**Total new code**: ~600 lines
**Files**: server/internal/transcription/rnnoise.go, server/internal/transcription/vad.go, server/internal/transcription/chunker.go, server/internal/transcription/pipeline.go, server/internal/config/config.go, server/config.example.yaml, server/cmd/server/main.go
---

