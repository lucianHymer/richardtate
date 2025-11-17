# ASR Interface Abstraction

**Status**: Phase 1 Complete ✅
**Last Updated**: 2025-11-17

## Overview
Clean abstraction layer for swappable Automatic Speech Recognition (ASR) engines. Enables supporting multiple transcription backends (Whisper, Parakeet, etc.) through a unified interface with zero changes to existing pipeline code.

## Purpose
Prepare for Parakeet integration while maintaining full backward compatibility with existing Whisper implementation. Provides factory pattern for runtime engine selection based on configuration.

## Architecture

### Core Interface
**Location**: `server/internal/transcription/asr_interface.go`

```go
type ASRTranscriber interface {
    Transcribe(samples []int16) (string, error)
    Close()
}
```

**Design Principles**:
- Minimal interface (2 methods only)
- Engine-agnostic (works for any ASR system)
- Simple types ([]int16 in, string out)
- Lifecycle management (Close for cleanup)

### Implementation Components

#### 1. ASR Interface (`asr_interface.go`)
Defines the contract all ASR engines must implement:
- `Transcribe(samples []int16)` - Convert audio to text
- `Close()` - Release resources

#### 2. Whisper Adapter (`whisper_adapter.go`)
Wraps existing `WhisperTranscriberShared` to implement ASRTranscriber interface:
- Zero functional changes to Whisper code
- Thin adapter layer (pass-through)
- Maintains all existing behavior

#### 3. Factory Pattern (`asr_factory.go`)
Creates appropriate ASR engine based on configuration:
- `NewASRTranscriber(config ASRConfig)` - Factory method
- Defaults to "whisper" for backward compatibility
- Ready for "parakeet" and other engines

#### 4. Pipeline Integration (`pipeline.go`)
Changed from concrete type to interface:
```go
// Before
transcriber *WhisperTranscriberShared

// After
transcriber ASRTranscriber
```

## Configuration

### ASRConfig Structure
```go
type ASRConfig struct {
    Engine             string              // "whisper" or "parakeet"
    SharedWhisperModel whisper.Model       // For whisper engine
    WhisperConfig      WhisperConfig       // Whisper-specific settings
}
```

**Design Decision**: Simplified config to avoid duplication. ASRConfig contains engine selection and delegates to engine-specific configs (WhisperConfig, future ParakeetConfig) rather than duplicating fields.

### Default Behavior
Engine defaults to "whisper" if not specified:
```go
if cfg.Engine == "" {
    cfg.Engine = "whisper"
}
```

**Backward Compatibility**: All existing code works unchanged. No config changes required.

## Implementation Details

### Whisper Adapter
**Location**: `server/internal/transcription/whisper_adapter.go`

```go
type WhisperAdapter struct {
    transcriber *WhisperTranscriberShared
}

func (w *WhisperAdapter) Transcribe(samples []int16) (string, error) {
    // Convert int16 to float32 (Whisper expects float32)
    floatSamples := convertPCMToFloat32(samples)
    return w.transcriber.Transcribe(floatSamples)
}

func (w *WhisperAdapter) Close() {
    w.transcriber.Close()
}
```

**Key Point**: Handles int16 → float32 conversion so pipeline always uses int16 (RNNoise output format).

### Factory Implementation
**Location**: `server/internal/transcription/asr_factory.go`

```go
func NewASRTranscriber(cfg ASRConfig) (ASRTranscriber, error) {
    engine := cfg.Engine
    if engine == "" {
        engine = "whisper"
    }

    switch engine {
    case "whisper":
        if cfg.SharedWhisperModel == nil {
            return nil, fmt.Errorf("SharedWhisperModel required for whisper engine")
        }
        transcriber, err := NewWhisperTranscriberShared(cfg.SharedWhisperModel)
        if err != nil {
            return nil, err
        }
        return &WhisperAdapter{transcriber: transcriber}, nil

    case "parakeet":
        // Phase 2: Implement ParakeetTranscriber
        return nil, fmt.Errorf("parakeet engine not yet implemented")

    default:
        return nil, fmt.Errorf("unknown ASR engine: %s", engine)
    }
}
```

### Pipeline Changes
**Location**: `server/internal/transcription/pipeline.go`

```go
// Changed field type from concrete to interface
type Pipeline struct {
    transcriber ASRTranscriber  // Was: *WhisperTranscriberShared
    // ...
}

// Factory call instead of direct construction
transcriber, err := NewASRTranscriber(ASRConfig{
    Engine:             "whisper",  // Could come from config
    SharedWhisperModel: config.SharedWhisperModel,
    WhisperConfig:      config.Whisper,
})
```

## Benefits

### 1. Engine Flexibility
- Swap ASR engines without touching pipeline code
- Runtime selection based on config
- Multiple engines supported simultaneously (future)

### 2. Backward Compatibility
- All existing Whisper code unchanged
- Adapter pattern preserves behavior
- Default to "whisper" if not specified
- Zero breaking changes

### 3. Clean Separation
- Interface defines contract
- Engines implement independently
- Pipeline agnostic to engine details
- Testable with mocks

### 4. Future Ready
- Parakeet integration prepared (Phase 2)
- Other engines easy to add
- Per-client engine selection possible
- A/B testing different engines

## Phase 1 Status

### ✅ Completed
- ASRTranscriber interface defined
- Whisper adapter implemented
- Factory pattern with engine selection
- Pipeline updated to use interface
- Backward compatibility verified
- Build tested with full CGO flags

### ✅ Verified
- Compiles cleanly with Whisper + RNNoise
- No functional changes to transcription
- All existing tests pass
- Zero config changes required

## Phase 2 Status

### ✅ Completed (2025-11-17)
- ParakeetTranscriber implementation with subprocess management
- Python worker scripts (real + mock) for platform flexibility
- ParakeetConfig structure for engine-specific configuration
- Factory pattern fully wired with Parakeet support
- Server config extended with engine selection
- Build script integration for automatic Parakeet MLX installation
- Full IPC protocol (JSON + Base64) with error handling

### ✅ Implementation Details
**ParakeetTranscriber** (`server/internal/transcription/parakeet_transcriber.go`):
- Subprocess lifecycle management (launch, monitor, shutdown)
- JSON + Base64 IPC protocol over stdin/stdout
- Thread-safe with mutex-protected communication
- Graceful shutdown with 5s force-kill timeout
- Platform detection: macOS uses real worker, Linux uses mock

**Worker Scripts**:
- `scripts/parakeet_worker.py` - Real Parakeet MLX implementation (macOS)
- `scripts/parakeet_mock.py` - Testing mock for Linux development

**Configuration** (server config.yaml):
```yaml
transcription:
  engine: "parakeet"  # or "whisper" (default: "whisper")
  model_path: "nvidia/parakeet-tdt-1.1b"  # Engine-specific
```

**Factory** (`asr_factory.go`):
```go
case "parakeet":
    return NewParakeetTranscriber(cfg.ParakeetConfig)
```

See [Parakeet Integration](parakeet-integration.md) for complete implementation details.

## Design Decisions

### Why Interface (Not Abstract Class)?
- Go idiom: Small interfaces
- Multiple implementations without inheritance
- Easy to mock for testing
- Minimal coupling

### Why Adapter (Not Modify Whisper)?
- Preserve existing Whisper code
- Separate concerns (ASR vs implementation)
- Easy to swap or remove
- No risk to working code

### Why Factory (Not Direct Construction)?
- Central engine selection logic
- Configuration-driven choice
- Easy to add new engines
- Consistent initialization

### Why int16 Interface (Not float32)?
- RNNoise outputs int16
- Most audio systems use int16
- Conversion localized to adapters
- Pipeline doesn't care about engine format

## Testing Strategy

### Unit Tests
- Mock ASRTranscriber for pipeline tests
- Test factory engine selection
- Verify adapter conversions
- Test error handling

### Integration Tests
- End-to-end with real Whisper
- Config-driven engine selection
- Multiple concurrent engines
- Resource cleanup verification

### Phase 2 Tests
- Parakeet subprocess lifecycle
- Mock worker for Linux testing
- Real worker for macOS
- Fallback behavior

## Known Limitations

1. **Single engine per pipeline**: Can't switch engines mid-session
2. **No partial results yet**: Interface returns complete string only
3. **Synchronous only**: No streaming partial results (future enhancement)

## Future Enhancements

### Streaming Interface
Support partial results:
```go
type StreamingASRTranscriber interface {
    ASRTranscriber
    TranscribeStream(samples []int16) (<-chan PartialResult, error)
}
```

### Multi-Engine Fallback
Try multiple engines if one fails:
```go
engines := []string{"parakeet", "whisper"}
for _, engine := range engines {
    result, err := transcriber.Transcribe(samples)
    if err == nil {
        return result
    }
}
```

### Per-Client Engines
Different engines for different clients:
```go
// In pipeline config
if client.PreferredEngine != "" {
    cfg.Engine = client.PreferredEngine
}
```

## Related Documentation
- [Whisper Model Sharing](whisper-model-sharing.md) - Shared model pattern
- [Transcription Pipeline](transcription-pipeline.md) - Pipeline implementation
- [Parakeet Integration](parakeet-integration.md) - Complete Parakeet implementation details

## Files
- `server/internal/transcription/asr_interface.go` - Interface definition
- `server/internal/transcription/whisper_adapter.go` - Whisper adapter
- `server/internal/transcription/asr_factory.go` - Factory pattern
- `server/internal/transcription/pipeline.go` - Pipeline integration
- `docs/PARAKEET_SUBPROCESS_IMPLEMENTATION.md` - Complete implementation plan
