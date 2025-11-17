### [23:32] [gotcha] Parakeet MLX Expects File Paths Not Audio Arrays
**Details**: **Discovered**: 2025-11-17

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
**Files**: scripts/parakeet_worker.py, scripts/requirements-parakeet.txt
---

