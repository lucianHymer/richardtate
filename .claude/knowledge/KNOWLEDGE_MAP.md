# Knowledge Map

**Last Updated**: 2025-11-17

This knowledge map provides a comprehensive index to all documented project knowledge. Each entry links to detailed documentation organized by category.

## Architecture

System design, component relationships, and structural decisions.

- [WebRTC Reconnection System](architecture/webrtc-reconnection-system.md) - Complete reconnection architecture with automatic recovery and 99% data integrity *(Updated: 2025-11-05)*
- [Transcription Pipeline Architecture](architecture/transcription-pipeline.md) - Real-time pipeline with RNNoise noise suppression, VAD-based smart chunking, and Whisper.cpp transcription *(Updated: 2025-11-06)*
- [Whisper Model Sharing](architecture/whisper-model-sharing.md) - ✅ Efficient memory usage with single shared model and multiple contexts *(Updated: 2025-11-17)*
- [ASR Interface Abstraction](architecture/asr-interface-abstraction.md) - ✅ Phases 1 & 2 Complete: Swappable ASR engines (Whisper, Parakeet) via unified interface *(Updated: 2025-11-17)*
- [Parakeet MLX Integration](architecture/parakeet-integration.md) - ✅ Phase 2 Complete: Parakeet ASR engine with subprocess management and MLX acceleration *(Updated: 2025-11-17)*
- [Per-Client Pipeline](architecture/per-client-pipeline.md) - Client-controlled transcription pipelines with custom VAD settings per connection *(Updated: 2025-11-06)*
- [VAD Calibration API](architecture/vad-calibration-api.md) - ✅ Implemented API-driven calibration with Hammerspoon wizard *(Updated: 2025-11-06, Session 15)*
- [Config Hot-Reload](architecture/config-hot-reload.md) - Automatic configuration reloading for calibration changes without daemon restart *(Updated: 2025-11-06, Session 16)*
- [Logging System Architecture](architecture/logging-system.md) - Unified structured logging system with log levels and structured fields *(Updated: 2025-11-06)*
- [Debug Log System](architecture/debug-log-system.md) - Persistent transcription logging with 8MB rolling rotation *(Updated: 2025-11-06)*
- [Hammerspoon Integration](architecture/hammerspoon-integration.md) - System-wide transcription with direct text insertion and visual calibration wizard *(Updated: 2025-11-06, Session 15)*

## Dependencies

External services, libraries, and third-party integrations.

- [Whisper.cpp and RNNoise Setup](dependencies/whisper-and-rnnoise-setup.md) - Installation scripts and configuration for Phase 2 transcription dependencies *(Updated: 2025-11-06)*
- [Whisper Initial Prompt Support](dependencies/whisper-initial-prompt.md) - Context prompting capabilities for improved domain-specific transcription accuracy *(Updated: 2025-11-06, Session 16)*

## Patterns

Coding patterns, conventions, and project-specific approaches.

*(No entries yet)*

## Workflows

How to perform common tasks and operations in this project.

- [Building with CGO Dependencies](workflows/building-with-cgo.md) - Critical workflows for building server with Whisper.cpp and RNNoise *(Updated: 2025-11-06)*
- [VAD Calibration Wizard](workflows/vad-calibration.md) - Interactive wizard for calibrating Voice Activity Detection threshold *(Updated: 2025-11-06)*

## Gotchas

Surprises, non-obvious behaviors, and things to watch out for.

- [Transcription Pipeline Gotchas](gotchas/transcription-gotchas.md) - Critical issues and non-obvious behaviors in transcription pipeline *(Updated: 2025-11-17)*

---

*This knowledge map is automatically maintained by the Mím knowledge system.*
