### [20:33] [gotcha] MLX Memory Leak - Must Clear Caches
**Details**: MLX (Apple's Machine Learning framework) aggressively caches intermediate computations for GPU performance. Without explicit cache clearing, memory grows unbounded over many transcription calls - users reported 30+ GB memory usage and crashes after hours of use.

**Solution**: Call `mx.clear_caches()` after each transcription:
```python
import mlx.core as mx

result = model.transcribe(audio_path)
text = result.text
mx.clear_caches()  # CRITICAL: prevents memory leak
```

Also run `gc.collect()` periodically (every ~10 transcriptions) to help Python release numpy arrays.

**Affected files**:
- scripts/parakeet_worker.py (batch mode)
- scripts/parakeet_worker_streaming.py (streaming mode)

**Root cause**: MLX caches tensor computations for potential reuse, but in a long-running transcription service, these are never reused and just accumulate.
**Files**: scripts/parakeet_worker.py, scripts/parakeet_worker_streaming.py
---

