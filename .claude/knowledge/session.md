### [13:37] [gotcha] Parakeet Final Word Cut-off Issue
**Details**: Parakeet streaming requires more silence to flush final words through its internal buffer. Originally, we only sent 1 second of silence after speech stopped, which caused ~50% of final words to be cut off. 

Fixed by:
1. Increasing silence accumulation from 1 second to 1.5 seconds (parakeetBufferSamples = 24000)
2. Adding explicit 2-second silence flush in pipeline.Stop() for Parakeet engine (was completely missing!)

The Stop() flush is critical because Parakeet doesn't have the same chunker.Flush() mechanism that Whisper uses. Without explicit silence on Stop(), any audio still in Parakeet's internal buffer is lost.
**Files**: server/internal/transcription/pipeline.go
---

