# Phase 2 Preparation: Whisper & RNNoise Integration

This document outlines the setup for Phase 2 transcription features.

## What We've Created

### Installation Scripts (`/scripts/`)

1. **`install-whisper.sh`** - Downloads and builds whisper.cpp
   - Clones from official repo: `github.com/ggml-org/whisper.cpp`
   - Builds `libwhisper.a` static library with CMake
   - Creates symlink for easy access
   - ~2-5 minutes on modern CPU

2. **`download-models.sh`** - Downloads Whisper GGML models
   - Default: `large-v3-turbo` (~1.6GB)
   - Source: Hugging Face (`ggerganov/whisper.cpp`)
   - Configurable for other models (tiny, base, small, medium, large)

3. **`download-rnnoise.sh`** - Downloads RNNoise noise suppression model
   - Model: "leavened-quisling" (lq.rnnn)
   - Source: GregorR/rnnoise-models
   - Used for real-time noise reduction

4. **`setup-env.sh`** - Sets CGO environment variables
   - Must be sourced before building: `source ./scripts/setup-env.sh`
   - Sets `CGO_CFLAGS`, `CGO_LDFLAGS`, `LIBRARY_PATH`
   - Required for Go to link against whisper.cpp

## Quick Setup

### For Development (This Machine - Linux)
```bash
./scripts/install-whisper.sh
./scripts/download-models.sh
./scripts/download-rnnoise.sh
source ./scripts/setup-env.sh
make build
```

### For Production (macOS with Apple Silicon)
```bash
brew install whisper-cpp
mkdir -p ~/.cache/whisper
curl -L -o ~/.cache/whisper/ggml-large-v3-turbo.bin \
  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin?download=true"
./scripts/download-rnnoise.sh
# Then build normally
```

## Dependencies for Phase 2

### Go Packages to Add

```bash
# Whisper.cpp Go bindings (official)
go get github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper

# RNNoise Go implementation (April 2025)
go get github.com/xaionaro-go/audio/pkg/noisesuppression/implementations/rnnoise

# Or alternative RNNoise wrapper
go get github.com/errakhaoui/noise-canceling
```

### System Libraries Required

**Linux:**
- CMake 3.10+
- C++ compiler (g++ or clang++)
- ALSA development libraries (`alsa-lib-devel` or `libasound2-dev`)

**macOS:**
- Homebrew
- Xcode Command Line Tools (for compilation if not using Homebrew)

## Phase 2 Implementation Tasks

### Server-Side (`/server/`)

1. **Create `/server/internal/transcription/`**
   - `whisper.go` - Whisper context management and transcription
   - `accumulator.go` - Chunk accumulation and VAD-based segmentation
   - `rnnoise.go` - Noise suppression preprocessing
   - `pipeline.go` - Audio processing pipeline coordinator

2. **Modify `/server/internal/api/server.go`**
   - Wire up transcription pipeline
   - Handle audio chunks → RNNoise → VAD → Whisper
   - Stream transcriptions back via DataChannel

3. **Update Configuration**
   - Add whisper model path to `server/config.yaml`
   - Add RNNoise model path
   - Configure VAD parameters (silence threshold, chunk duration)

### Client-Side (`/client/`)

1. **Modify `/client/cmd/client/main.go`**
   - Handle `MessageTypeTranscriptionChunk` from server
   - Accumulate transcription chunks
   - Log all transcriptions to debug log

2. **Add Debug Logging**
   - Implement 8MB rolling log at `~/.streaming-transcription/debug.log`
   - Log format: timestamped JSON
   - Log each transcription chunk as received

## Technical Specifications

### Audio Pipeline (Phase 2)
```
Client captures audio (16kHz mono PCM, 200ms chunks)
         ↓
Send via DataChannel to server
         ↓
Server: RNNoise processing (10ms frames)
         ↓
VAD: Detect speech/silence boundaries
         ↓
Accumulate chunks until 500-800ms silence
         ↓
Whisper transcription (per accumulated segment)
         ↓
Send transcription back via DataChannel
         ↓
Client displays and logs transcription
```

### Key Numbers
- **Audio capture**: 16kHz, mono, 16-bit PCM
- **Chunk size**: 200ms = 6400 bytes raw PCM
- **RNNoise frame**: 10ms = 160 samples at 16kHz
- **VAD silence threshold**: 500-800ms (configurable)
- **Whisper context**: Preserve between segments for better accuracy

## Testing Strategy

### Unit Tests Needed
- RNNoise frame processing
- VAD state machine (speech/silence detection)
- Chunk accumulation logic
- Whisper context management
- Transcription accuracy

### Integration Tests
- End-to-end: audio → RNNoise → VAD → Whisper → client
- Verify transcription latency < 500ms
- Test with various noise conditions
- Test with different speaking speeds
- Verify no audio chunks are lost

## Known Considerations

### From Implementation Plan

1. **Reliable DataChannel ensures no lost audio**
   - We already have this working from Phase 1
   - All audio chunks arrive in order
   - Transcription will process complete audio

2. **Chunk accumulation uses VAD**
   - Don't send every 200ms chunk to Whisper
   - Accumulate until natural pause (500-800ms silence)
   - Better transcription quality vs. word-by-word

3. **RNNoise operates on 10ms frames**
   - Our 200ms chunks = 20 RNNoise frames
   - Process each frame individually
   - Maintain state between frames

4. **Whisper context preservation**
   - Keep context from previous segments
   - Improves accuracy across segment boundaries
   - Helps with proper capitalization and flow

## Environment Variables Reference

```bash
# Whisper.cpp paths
export WHISPER_DIR="/workspace/project/deps/whisper.cpp"
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build -lwhisper"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
export LIBRARY_PATH="$WHISPER_DIR/build:${LIBRARY_PATH}"
export LD_LIBRARY_PATH="$WHISPER_DIR/build:${LD_LIBRARY_PATH}"
```

## File Locations

```
/workspace/project/
├── deps/
│   └── whisper.cpp/          # Whisper.cpp source and build
│       ├── include/          # Header files
│       ├── ggml/include/     # GGML header files
│       └── build/
│           └── libwhisper.a  # Static library
├── models/
│   ├── ggml-large-v3-turbo.bin  # Whisper model
│   └── rnnoise/
│       └── lq.rnnn           # RNNoise model
└── scripts/
    ├── install-whisper.sh
    ├── download-models.sh
    ├── download-rnnoise.sh
    └── setup-env.sh
```

## Next Steps

1. **Test the installation scripts** on this machine
2. **Add Go dependencies** for Whisper and RNNoise
3. **Create transcription module** structure
4. **Implement RNNoise preprocessing**
5. **Implement VAD logic**
6. **Integrate Whisper**
7. **Test end-to-end transcription**

## Resources

- **Whisper.cpp repo**: https://github.com/ggml-org/whisper.cpp
- **Go bindings docs**: https://pkg.go.dev/github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper
- **RNNoise Go**: https://pkg.go.dev/github.com/xaionaro-go/audio/pkg/noisesuppression/implementations/rnnoise
- **RNNoise models**: https://github.com/GregorR/rnnoise-models
- **Hugging Face models**: https://huggingface.co/ggerganov/whisper.cpp

---

**Status**: Ready for Phase 2 implementation. All setup scripts and documentation complete.
