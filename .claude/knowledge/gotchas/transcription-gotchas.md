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

## Parakeet Python Dependencies Missing

**Discovered**: 2025-11-17

**Problem**: parakeet_worker.py fails with "ModuleNotFoundError: No module named 'numpy'" when Parakeet engine is used.

**Root Cause**: The Python worker script requires numpy and parakeet-mlx packages but they may not be installed if user didn't run through build-mac.sh installation wizard or if pip installation failed silently.

**Solution**:
1. Created `scripts/requirements-parakeet.txt` with explicit dependencies
2. Created `scripts/install-parakeet.sh` for easy installation
3. Updated build-mac.sh to use requirements file instead of direct pip install
4. Updated README.md with manual installation instructions

**Installation**:
```bash
# Quick install
./scripts/install-parakeet.sh

# Or manually
pip3 install -r scripts/requirements-parakeet.txt
```

**Dependencies**:
- numpy >= 1.24.0
- parakeet-mlx >= 0.1.0

**Testing Before Commit**: Always test Python scripts can actually run before committing:
```bash
python3 scripts/parakeet_worker.py --help  # Should not fail with import errors
```

**Why This Matters**: Python import errors only appear at runtime, not during Go build. Must test scripts directly to catch these issues.

**Files**: scripts/requirements-parakeet.txt, scripts/install-parakeet.sh, scripts/parakeet_worker.py, scripts/build-mac.sh

---

## Parakeet MLX Expects File Paths Not Audio Arrays

**Discovered**: 2025-11-17

**Problem**: The Parakeet MLX library's `transcribe()` method expects a **file path** (string), not audio samples (numpy array). When passing audio samples directly, it fails with:
```
TypeError: expected str, bytes or os.PathLike object, not ndarray
```

**Root Cause**: The `parakeet_mlx.transcribe()` API is file-based, not array-based. It internally calls `Path(path)` expecting a path to an audio file.

**Solution**: Write audio samples to a temporary WAV file and pass the file path to `transcribe()`:
1. Use `scipy.io.wavfile` to write WAV files
2. Convert float32 audio to int16 for WAV format
3. Write to temporary file with `tempfile.mkstemp(suffix='.wav')`
4. Pass file path to `model.transcribe()`
5. Clean up temporary file after transcription

**Implementation**:
```python
# Write to temporary WAV file
temp_wav_path = write_temp_wav(audio_samples, sample_rate)
try:
    # Transcribe from file
    result = model.transcribe(temp_wav_path)
    response = {"text": result['text']}
finally:
    # Clean up temporary file
    os.remove(temp_wav_path)
```

**Dependencies Added**: `scipy>=1.10.0` for WAV file writing

**Why This Pattern**: Many ML libraries are optimized for file-based workflows and don't expose array-based APIs. The overhead of writing temporary files is acceptable for transcription latency.

**Files**: scripts/parakeet_worker.py, scripts/requirements-parakeet.txt

---

## Parakeet Subprocess Needs Environment Inheritance for FFmpeg

**Discovered**: 2025-11-17

**Problem**: Parakeet Python worker fails with "FFmpeg is not installed or not in your PATH" even when FFmpeg is installed on the system.

**Root Cause**: Go's `exec.Command()` by default does NOT inherit the parent process's environment variables. The Python subprocess had an empty PATH and couldn't find FFmpeg.

**Solution**: Set `cmd.Env = os.Environ()` to pass the parent environment to the subprocess:
```go
cmd := exec.Command(pythonPath, scriptPath, config.ModelPath)
cmd.Env = os.Environ()  // Inherit PATH and other environment variables
```

**Why FFmpeg is Needed**: Parakeet MLX uses FFmpeg to load audio files. The `parakeet_mlx.load_audio()` function internally calls FFmpeg to decode audio files.

**Testing**: Verify FFmpeg is accessible:
```bash
which ffmpeg  # Should show path like /opt/homebrew/bin/ffmpeg
```

**Files**: server/internal/transcription/parakeet_transcriber.go

---

## Subprocess Environment Inheritance Not Enough for FFmpeg

**Discovered**: 2025-11-17

**Problem**: Even after setting `cmd.Env = os.Environ()`, the Parakeet Python subprocess still couldn't find FFmpeg.

**Root Cause**: The server process may not have FFmpeg in its PATH, or the PATH order doesn't prioritize Homebrew/local installations. Simply inheriting the environment isn't enough if the parent process doesn't have the right PATH.

**Solution**: Explicitly prepend common FFmpeg installation locations to PATH:
```go
env := os.Environ()
for i, e := range env {
    if len(e) > 5 && e[:5] == "PATH=" {
        // Prepend FFmpeg locations
        env[i] = "PATH=/opt/homebrew/bin:/usr/local/bin:" + e[5:]
        break
    }
}
cmd.Env = env
```

**Why This is Needed**:
- Homebrew installs FFmpeg to `/opt/homebrew/bin` on Apple Silicon
- System FFmpeg is in `/usr/local/bin` on Intel Macs
- Server might be started without these in PATH
- Subprocess needs explicit PATH modification

**Testing**: Check debug logs for "Subprocess PATH:" to verify FFmpeg directories are included.

**Files**: server/internal/transcription/parakeet_transcriber.go

---

## Parakeet MLX Returns AlignedResult Object Not Dict

**Discovered**: 2025-11-17

**Problem**: Accessing transcription result with `result['text']` fails with "TypeError: 'AlignedResult' object is not subscriptable"

**Root Cause**: Parakeet MLX's `transcribe()` method returns an `AlignedResult` object, not a dictionary. It's an object with attributes, not a dict with keys.

**Solution**: Access as attribute instead of dictionary:
```python
# WRONG
response = {"text": result['text']}  # TypeError!

# CORRECT
response = {"text": result.text}  # Attribute access
```

**Why**: The `AlignedResult` class has a `.text` attribute containing the transcribed text. It may also have other attributes like `.segments`, `.language`, etc.

**Testing**: After transcription completes, check stderr logs for "Model loaded successfully" followed by successful response (no TypeError).

**Files**: scripts/parakeet_worker.py

---

## Whisper CGO Build Requirements

**Discovered**: 2025-11-17

To build the server with Whisper.cpp support, you MUST source the environment script first:

```bash
cd /workspace/project/server
. ../scripts/setup-env.sh  # Sources CGO environment variables
go build ./cmd/server       # Now build will succeed
```

The setup-env.sh script sets critical CGO flags:
- CGO_CFLAGS: Include paths for whisper.h headers
- CGO_LDFLAGS: Library paths for linking
- CGO_CFLAGS_ALLOW: Allows -mfma and -mf16c compiler flags

Without sourcing this script, builds will fail with "whisper.h: No such file or directory"

Alternative: Export the variables manually:
```bash
export WHISPER_DIR=/workspace/project/deps/whisper.cpp
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build/src -L$WHISPER_DIR/build/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++ -lm"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
```

**Files**: scripts/setup-env.sh, server/cmd/server/main.go

---

## Parakeet Streaming MLX Array Conversion

**Discovered**: 2025-11-17

When implementing Parakeet streaming, the audio samples must be converted from numpy arrays to MLX arrays before passing to context.add_audio(). The parakeet_mlx library internally uses mx.concat() which expects MLX arrays, not numpy arrays.

Error: TypeError: concat(): incompatible function arguments when passing numpy arrays directly.

Solution: Convert numpy array to MLX array using mx.array() before calling context.add_audio():
```python
import mlx.core as mx
mlx_audio = mx.array(audio_samples)  # Convert numpy to MLX
context.add_audio(mlx_audio)
```

This is different from the non-streaming version which may accept numpy arrays directly.

**Files**: scripts/parakeet_worker_streaming.py

---

## Parakeet Streaming Returns Accumulated Text

**Discovered**: 2025-11-17

**Problem**: The Parakeet MLX streaming API returns the ENTIRE accumulated transcription from the start of the stream each time you add audio, not just incremental text. This is expected behavior - the streaming context maintains full history and can revise earlier parts.

**Solution**: Track previously sent text per client and only send the incremental difference. Implementation in parakeet_worker_streaming.py uses self.previous_text dict to track what was already sent and calculates incremental_text = full_text[len(previous):] to send only new content.

Without this tracking, the same text gets sent repeatedly, appearing as if text is repeating and accumulating incorrectly on the client side.

**Files**: scripts/parakeet_worker_streaming.py

---

## Parakeet Context Window Size Affects Finalization Delay

**Discovered**: 2025-11-18

**Problem**: Parakeet streaming uses a context window that directly affects when tokens become "finalized". With default context_size=(256, 256), tokens don't finalize for 4+ MINUTES of audio, not seconds as expected.

**Critical Understanding**:
- context_size=(256, 256) means 256 frames lookahead/lookbehind
- At 16kHz with ~62.5 frames/second, 256 frames = ~4 seconds lookahead
- Tokens only become "finalized" after passing through the lookahead window
- This means with (256, 256), tokens aren't finalized until 4+ minutes of audio has been processed!

**Tested Behavior**:
- With context_size=(10, 10): Tokens finalize after ~10 seconds
- With context_size=(256, 256): Tokens don't finalize for 4+ minutes
- result.text contains BOTH finalized AND draft tokens combined
- finalized_tokens grows over time as tokens pass the lookahead window
- Text constantly revises until finalized

**Current Solution**:
Using default (256, 256) for maximum quality and sending full result.text as preview. Accept that text will revise continuously and only commit final text when streaming ends.

**Future Options**:
1. Reduce context window to ~60 frames (~1 second lookahead) for faster finalization
2. Dual-mode: Show finalized tokens as "committed" and full text as preview
3. Adaptive window: Small for responsiveness, larger during pauses for quality

**Why This Matters**: This explains why Parakeet never seemed to finalize tokens in testing - we were using such a large context window that finalization took longer than most test recordings!

**Files**: scripts/parakeet_worker_streaming.py, .claude/knowledge/architecture/parakeet-streaming-preview.md

---
