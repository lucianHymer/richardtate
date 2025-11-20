# Testing Parakeet Final Word Flush

## Test Plan

### What Was Fixed

1. **Increased silence buffer duration**: Changed from 1 second to 1.5 seconds
   - This gives more time for Parakeet's internal buffer to process and output the final word
   - Located in pipeline.go line 178: `const parakeetBufferSamples = 24000 // 1.5 seconds`

2. **Added explicit flush on Stop()**: When recording stops, we now send 2 seconds of silence
   - This ensures any remaining audio in Parakeet's buffer gets flushed out
   - Located in pipeline.go lines 347-366

### Test Scenarios

1. **Quick utterance test**:
   - Say: "Testing one two three" and immediately stop
   - Expected: All words should appear, including "three"

2. **Trailing word test**:
   - Say: "The quick brown fox jumps over the lazy dog"
   - Stop immediately after "dog"
   - Expected: "dog" should appear

3. **Multiple short phrases**:
   - Say: "Hello" (pause) "World" (pause) "Goodbye"
   - Stop after "Goodbye"
   - Expected: All three words should appear

### Configuration to Test

```yaml
transcription:
  engine: "parakeet"
  model_path: "mlx-community/parakeet-tdt-0.6b-v3"
```

### Expected Behavior

- During speech: Real-time streaming as before
- After speech stops: ~1.5 second delay before text appears (was 1 second)
- When Stop pressed: Final flush should capture any remaining words within ~2 seconds

### Debug Logging

Watch for these log messages:
- `"Final Parakeet flush result: [text]"` - Shows what was recovered during Stop()
- This appears in pipeline.go line 362

## Summary of Changes

The issue was that Parakeet's internal buffering needs more silence to push through the final word(s). We addressed this in two ways:

1. **During recording**: Increased the silence accumulation from 1s to 1.5s before sending
2. **On Stop()**: Added explicit 2-second silence flush for Parakeet (was missing entirely!)

These changes should eliminate the ~50% loss rate of final words you were experiencing.