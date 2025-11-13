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

