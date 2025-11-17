### [21:35] [architecture] Whisper Model Sharing Architecture
**Details**: Critical architecture issue discovered: Each pipeline was creating its own Whisper model (1.6GB each) instead of sharing a single model across contexts. This caused massive memory usage (14-15GB with multiple connections).

Correct architecture:
1. Load Whisper model ONCE at server startup
2. Pass the model (not model path) to pipelines
3. Each pipeline creates its own context from the shared model
4. Model lives for entire server lifetime

Whisper.cpp is designed for this - one model can have many contexts for concurrent transcription. Each context is lightweight (~few MB) while the model is heavyweight (1.6GB).

Key insight: whisper.Model and whisper.Context are separate. Model = weights/parameters (shared), Context = processing state (per-session).
**Files**: server/internal/transcription/whisper.go, server/internal/transcription/pipeline.go, server/internal/webrtc/manager.go
---

### [19:01] [architecture] ASR Interface Abstraction - Phase 1 Complete
**Details**: Phase 1 of Parakeet integration completed 2025-11-17. Implemented clean ASR interface abstraction layer to support swappable speech recognition engines (Whisper, Parakeet, etc.).

Implementation:
- asr_interface.go: ASRTranscriber interface with Transcribe() and Close() methods
- whisper_adapter.go: Adapter wrapping existing WhisperTranscriberShared with zero functional changes
- asr_factory.go: Factory pattern with NewASRTranscriber(), defaults to "whisper" for backward compatibility
- pipeline.go: Changed from concrete *WhisperTranscriberShared to ASRTranscriber interface

Key design decision: Simplified ASRConfig structure to avoid duplication with existing WhisperConfig. ASRConfig just contains Engine string, SharedWhisperModel pointer, and WhisperConfig struct. Cleaner separation of concerns.

Backward compatibility guaranteed: Engine defaults to "whisper" if not specified. All existing code works unchanged. No config changes required.

Build verified: Compiles cleanly with full CGO flags (Whisper + RNNoise).

Phase 2 ready: Factory has commented placeholder for "parakeet" case. Need to implement parakeet_transcriber.go (subprocess manager), parakeet_worker.py (real worker), parakeet_mock.py (Linux mock), and config wiring.

Documentation: Updated docs/PARAKEET_SUBPROCESS_IMPLEMENTATION.md with complete Phase 1 status, deviations, and handoff notes at top of file.
**Files**: server/internal/transcription/asr_interface.go, server/internal/transcription/whisper_adapter.go, server/internal/transcription/asr_factory.go, server/internal/transcription/pipeline.go, docs/PARAKEET_SUBPROCESS_IMPLEMENTATION.md
---

