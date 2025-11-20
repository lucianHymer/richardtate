### [13:37] [gotcha] Parakeet Final Word Cut-off Issue
**Details**: Parakeet streaming requires more silence to flush final words through its internal buffer. Originally, we only sent 1 second of silence after speech stopped, which caused ~50% of final words to be cut off. 

Fixed by:
1. Increasing silence accumulation from 1 second to 1.5 seconds (parakeetBufferSamples = 24000)
2. Adding explicit 2-second silence flush in pipeline.Stop() for Parakeet engine (was completely missing!)

The Stop() flush is critical because Parakeet doesn't have the same chunker.Flush() mechanism that Whisper uses. Without explicit silence on Stop(), any audio still in Parakeet's internal buffer is lost.
**Files**: server/internal/transcription/pipeline.go
### [13:28] [gotcha] Native UI migration: Timing change in silence/flush behavior
**Details**: DISCOVERED ISSUE: The migration from Hammerspoon to native Go UI introduced a timing problem with transcription streaming that makes silence handling inconsistent.

**Root Cause**: Timing between audio capture stop and control.stop message

**Before (Hammerspoon)**:
- Recording happens through HTTP /start endpoint
- Recording stop happens through HTTP /stop endpoint
- Both are synchronous and immediate

**After (Native UI)**:
- `startRecording()` in ui.go calls `onStart()` handler (which sends control.start over DataChannel)
- `stopRecording()` in ui.go calls `onStop()` handler (which sends control.stop over DataChannel)
- BUT there's a race condition: audio is still flowing on the DataChannel when control.stop arrives!

**Critical Flow Issue** (client/cmd/main.go lines 190-223):
1. User presses Ctrl+N (stop)
2. `stopRecording()` is called (ui.go line 199)
3. `onStop()` handler is called (line 214)
4. Audio capture is STOPPED FIRST (line 195)
5. Debug log is written (lines 201-215)
6. THEN control.stop is sent (line 218)

**Server Side Problem** (server/internal/api/server.go lines 265-276):
1. Server receives control.stop message
2. Immediately calls `pipeline.Stop()` (line 271)
3. `pipeline.Stop()` calls `chunker.Flush()` (server/internal/transcription/pipeline.go line 345)
4. `chunker.Flush()` applies speech duration checks (chunker.go lines 175-212)

**The Issue**: When control.stop arrives, the chunker may be in the middle of accumulating silence for VAD detection. The flush happens BEFORE the full 1-second silence threshold is reached, meaning:
- Final chunks may be discarded due to insufficient silence duration
- Streaming feels "cut off" compared to Hammerspoon where silence accumulated properly
- No time for VAD to properly detect silence boundaries

**Comparison with Hammerspoon**:
- Hammerspoon was receiving complete silence periods before any client intervention
- The stop command came as a separate HTTP request that could queue behind audio processing
- Server had time to process full silence periods before flush was triggered

**Key Code Locations**:
- Client stop handler: client/cmd/main.go lines 190-224
- Server stop handler: server/internal/api/server.go lines 265-276
- Pipeline stop: server/internal/transcription/pipeline.go lines 334-355
- Chunker flush: server/internal/transcription/chunker.go lines 173-212

**Why Transcriptions are "Shittier"**:
1. Streaming chunks arrive but final silence period is cut short
2. Whisper doesn't have enough buffered silence to finalize chunks properly
3. VAD never completes its silence threshold detection
4. Flush happens prematurely with insufficient speech duration checks satisfied
5. Result: incomplete/truncated transcriptions compared to Hammerspoon

**Possible Solutions**:
1. Add delay after audio capture stops before sending control.stop (gives VAD time to process final silence)
2. Change flush behavior to be more lenient when explicitly stopped (vs. auto-chunking)
3. Buffer final audio after stop signal for VAD processing
4. Change control.stop to trigger async flush rather than immediate flush
**Files**: client/cmd/client/main.go, client/internal/ui/ui.go, server/internal/api/server.go, server/internal/transcription/pipeline.go, server/internal/transcription/chunker.go
---

### [13:36] [gotcha] darwinkit objc.Retain causes double-free on window close
**Details**: When using objc.Retain() on a darwinkit window, you MUST call Release() before Close() to prevent a double-free crash. objc.Retain() sets up a Go finalizer that will call Release() when GC collects the object. If you close the window without releasing first, the finalizer will later try to release an already-deallocated object, causing SIGSEGV. The crash manifests as: "SIGSEGV: segmentation violation, signal arrived during cgo execution" with stack trace showing appkit.(*Window).Release called from runtime.runfinq(). Fix: In the Close() method, call w.nsWindow.Release() before w.nsWindow.Close().
**Files**: client/internal/ui/window.go
---

