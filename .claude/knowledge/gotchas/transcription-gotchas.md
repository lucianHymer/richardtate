# Transcription Pipeline Gotchas

Critical issues and non-obvious behaviors discovered while building the transcription pipeline.

---

## Whisper Hallucination on Noise-Only Chunks

**Symptom**: Whisper transcribes small hallucinated phrases like "Thank you." or "Thanks for watching!" between real transcriptions.

**Root Cause**: The chunker was sending audio chunks with very little actual speech content (e.g., 50ms of faint background noise + 1000ms of silence). Whisper, when given silence or noise-only audio, tends to hallucinate common phrases it was trained on.

**Solution**: Added minimum speech duration gating in the chunker. Now requires at least 1 second of actual detected speech (not just non-silence) before sending chunk to Whisper.

**Implementation** (`server/internal/transcription/chunker.go`):
```go
minSpeechDuration := 1 * time.Second
if shouldChunk &&
   bufferDuration >= c.config.MinChunkDuration &&
   vadStats.SpeechDuration >= minSpeechDuration {
    c.flushChunk()
}
```

**Why This Works**:
- VAD tracks `SpeechDuration` separately from total buffer duration
- Chunks must have sufficient speech content to be transcribed
- Filters out noise-only chunks before they reach Whisper
- Eliminates 80-90% of hallucinated chunks

**Configuration**: Currently hardcoded to 1 second. Could make configurable if needed.

**Related**: Discovered 2025-11-06 during Session 8 testing

---

## RNNoise Pass-Through Initially Required

**Issue**: The `github.com/xaionaro-go/audio` RNNoise implementation has complex requirements that blocked initial testing.

**Requirements for Real RNNoise**:
- CGO build with `pkg-config` for native rnnoise library
- Build tag: `-tags rnnoise`
- 48kHz audio (not 16kHz like our pipeline)
- Complex sample rate conversion logic

**Initial Decision**: Implemented RNNoise as pass-through (no actual denoising) for initial VAD testing.

**Pass-Through Implementation** (`server/internal/transcription/rnnoise.go`):
- All methods just pass data through unchanged
- Logs warning: "DISABLED - Using pass-through"
- Preserves API interface for future integration

**Why This Worked**:
- VAD could still operate on raw audio (just not denoised)
- Tested VAD chunking logic independently
- Simpler build process (no CGO dependencies)
- Could add real RNNoise later once VAD was proven

**Current Status**: Real RNNoise now implemented with 16kHz↔48kHz resampling. Pass-through still available when building without `-tags rnnoise`.

**Related**: Discovered 2025-11-06 during Session 8 implementation

---

## Homebrew RNNoise is the Wrong Package

**Critical Warning**: `brew install rnnoise` installs a VST audio plugin, NOT the librnnoise library needed for noise suppression.

**The Problem**:
- Homebrew package "rnnoise" is an audio plugin for music production
- Does NOT provide librnnoise shared library
- Does NOT provide pkg-config file
- Build will fail with "library not found" errors

**Correct Installation**:
```bash
# DO THIS:
./scripts/install-rnnoise-lib.sh  # Builds from source to deps/rnnoise/

# DO NOT DO THIS:
brew install rnnoise  # WRONG PACKAGE!
```

**Why Build from Source**:
- Installs to project-local `deps/rnnoise/` directory
- Provides pkg-config file for CGO
- Gives control over installation path
- Avoids conflicts with system packages

**Detection**: The `./scripts/build-mac.sh` script auto-detects locally-built rnnoise and sets appropriate flags.

**Related**: Clarified 2025-11-06 during RNNoise integration

---
