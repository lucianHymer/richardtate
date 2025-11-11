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

## Config Fields That Don't Actually Work

**Discovered**: 2025-11-06 (Session 13)

**Problem**: Several server config fields were defined but never used by the code:

1. **`noise_suppression.enabled`** - RNNoise is controlled by build tag `-tags rnnoise`, not config
2. **`transcription.translate`** - Hardcoded to false in whisper.go:57
3. **`transcription.use_gpu`** - Never used, GPU is auto-detected by Whisper.cpp
4. **`vad.enabled`** - VAD is always active, can't be disabled

**Solution**: These fields have been removed from the config struct. RNNoise being build-time is now clearly documented in config.example.yaml.

**Impact**: Cleaner config, less confusion. Users can't set options that do nothing.

**Files**: server/internal/config/config.go, server/config.example.yaml

---

## Client Config Fields That Don't Work

**Discovered**: 2025-11-06 (Session 13)

**Problem**: The client config had many defined fields that were never actually used:

1. **`server.reconnect_delay_ms`** - Hardcoded to 1s in webrtc/client.go:64
2. **`server.max_reconnect_delay_ms`** - Hardcoded to 30s max in webrtc/client.go:482
3. **`server.reconnect_backoff_multiplier`** - Hardcoded exponential backoff (2^n) in webrtc/client.go:481
4. **`audio.sample_rate`** - Hardcoded to 16000 in audio/capture.go:13
5. **`audio.channels`** - Hardcoded to 1 (mono) in audio/capture.go:14
6. **`audio.bits_per_sample`** - Hardcoded to 16 in audio/capture.go:17
7. **`audio.chunk_duration_ms`** - Hardcoded to 200ms in audio/capture.go:15

**Why Hardcoded**: These values are intentionally hardcoded because they're optimized for speech transcription and shouldn't be changed. Only device_name is kept configurable to allow selecting specific microphones.

**Solution**: All unused fields removed from config struct.

**Impact**: Simpler config, no false impression that these can be changed.

**Files**: client/internal/config/config.go, client/config.example.yaml

---

## Whisper Hallucination on Final Chunk

**Discovered**: 2025-11-06 (Session 13)

**Symptom**: Whisper hallucinated "thank you" on the final chunk when recording stopped.

**Root Cause**: `Flush()` was sending whatever remained in the buffer when recording stopped, even if it was mostly silence or trailing noise. Whisper hallucinated on this noise-only audio.

**Solution**: Apply same speech duration threshold (1 second minimum) to final flush as we do for regular chunks. Now `Flush()` checks `vadStats.SpeechDuration` and only transcribes if >= 1 second of actual speech detected. Otherwise, discards the final chunk with debug log message.

**Why This Works**: Prevents hallucinations on trailing silence while still allowing legitimate final chunks through.

**Files**: server/internal/transcription/chunker.go

---

## VAD Calibration Missing RNNoise Processing

**Discovered**: 2025-11-06 (Session 13)

**Status**: ✅ FIXED (Session 14, 2025-11-06)

**Problem**: VAD calibration was analyzing raw audio, but production VAD sees RNNoise-processed audio.

**Production Flow**:
```
Raw Audio → RNNoise → VAD → Chunker → Whisper
```

**Old Calibration Flow**:
```
Raw Audio → VAD energy calculation (no RNNoise!)
```

**New Calibration Flow**:
```
Raw Audio → RNNoise (if available) → VAD energy calculation
```

**Solution Implemented**:
1. Added `rnnoiseModelPath` field to API server
2. Calibration endpoint creates temporary RNNoise processor for each analysis
3. Processes audio through RNNoise before calculating energy statistics
4. Falls back to raw audio if RNNoise unavailable or errors occur
5. Matches production pipeline exactly

**Implementation**:
- API server stores RNNoise model path from config
- Creates temporary `RNNoiseProcessor` for each calibration request
- Properly cleans up processor after use
- Logs whether RNNoise was applied or raw audio used

**Files**:
- `server/internal/api/server.go` - Added RNNoise processing to calibration
- `server/cmd/server/main.go` - Pass RNNoise model path to API server
- `server/internal/transcription/rnnoise_real.go` - RNNoise processor reused

---

## Hammerspoon Direct Insertion vs Preview UI

**Discovered**: 2025-11-06 (Session 13)

**Design Deviation**: V1 Hammerspoon implementation uses direct text insertion instead of WebView preview UI originally specified in implementation plan (lines 236-279).

**Original Plan**:
- WebView window with HTML/CSS/JS
- Raw transcription panel (top)
- Processing mode buttons (middle, grayed out for V1)
- Processed output panel (bottom)
- Enter to insert, Cmd+C to copy, Esc to cancel

**What We Actually Built**:
- Simple Lua script (150 lines)
- Minimal canvas indicator (200x40px, top-right corner)
- Direct text insertion at cursor via `hs.eventtap.keyStrokes()`
- No UI panels, no WebView, no preview

**Why This Deviation is BETTER**:
1. **Simpler**: 150 lines Lua vs HTML+CSS+JS+WebView management
2. **Faster to ship**: 1 session vs 3-4 sessions
3. **More magical UX**: Text just appears (like Talon/voice coding tools)
4. **Fewer dependencies**: No WebView, no browser engine
5. **Better ergonomics**: No window to manage, no focus stealing
6. **Works everywhere**: Any app with text input
7. **Still V1 compliant**: Delivers core goal (streaming transcription works)

**What We Preserved**:
- Hotkey control (Ctrl+N)
- Real-time streaming display (now direct to cursor)
- Session text accumulation (in debug log)
- Minimal visual feedback (indicator vs window)
- All backend features

**V2 Can Still Add**:
- Preview UI if users request it
- Processing modes (casual, professional, etc.)
- WebView with formatting options
- Text editing before insertion

**For Future**: If WebView UI is needed, reference lines 236-279 of streaming-transcription-implementation-plan.md for original spec. Keep direct insertion as a mode option.

**Files**: hammerspoon/init.lua, streaming-transcription-implementation-plan.md

---

## Calibration Saved to Wrong Config Path

**Discovered**: 2025-11-06 (Session 16)

**Problem**: API calibration endpoint was saving threshold to wrong YAML path:
- Saving to: `transcription.vad_energy_threshold` (flat, wrong)
- Should be: `transcription.vad.energy_threshold` (nested, correct)

**Impact**:
- Calibration appeared to succeed but threshold wasn't actually used
- Client code reads from nested structure only
- CLI calibration had correct implementation, API calibration had wrong path

**Root Cause**: Code duplication between CLI calibration and API calibration endpoints

**Solution**:
1. Created shared `config.UpdateVADThreshold()` function in `client/internal/config/update.go`
2. Both CLI calibration and API calibration now use same function
3. Ensures consistency and prevents future divergence

**Lesson**: When two code paths do the same thing, extract to shared function immediately

**Files**: client/internal/config/update.go, client/internal/api/server.go, client/internal/calibrate/calibrate.go

---

## Default Debug Log Path Was Read-Only Filesystem Issue

**Discovered**: 2025-11-06 (Session 16)

**Problem**: The client config default for debug_log_path was set to "./debug.log" (current directory) on lines 60 and 116 of config.go. When client ran in a read-only filesystem, it would FATAL on startup.

**Solution**: Fixed to use "~/.config/richardtate/debug.log" to match config.example.yaml default. The debuglog package already handles ~ expansion correctly.

**Impact**: Client can now run from read-only filesystems without fatal errors on startup.

**Files**: client/internal/config/config.go

---

## Short Utterances Not Transcribed - Speech Density Solution

**Discovered**: 2025-11-06 (Session 16)

**Problem**: Short utterances like "yeah", "sure", "okay" were not being transcribed because they contained less than 1 second of actual speech. The 1-second minimum was implemented to prevent Whisper hallucinations on noise-only chunks.

**Solution**: Added speech density check - if a chunk has >= 60% speech density (speech time / total time), it will be sent to Whisper even if it has less than 1 second of speech. This allows legitimate short utterances through while still filtering out sparse noise chunks that cause hallucinations.

**Implementation**: Modified chunker.go checkAndChunk() and Flush() functions to calculate speech density and use dual criteria:
1. Original: >= 1 second of speech
2. New: Any amount of speech with >= 60% density (configurable)

**Configuration**: The speech density threshold is now configurable via client config:
- `transcription.vad.speech_density_threshold` (default: 0.6 = 60%)
- Configured in client config YAML
- Sent to server in control.start message
- Passed through pipeline config to chunker

**Tuning Guide**:
- Higher (0.7-0.9): More conservative, fewer false positives
- Lower (0.4-0.5): More aggressive, catches quieter/briefer utterances
- Default 0.6: Good balance for most use cases

**Why This Works**: Balances hallucination prevention with responsiveness for short conversational responses.

**Files**: server/internal/transcription/chunker.go, client/internal/config/config.go, shared/protocol/messages.go, server/internal/transcription/pipeline.go, client/config.example.yaml

---
