# Whisper Model Sharing Architecture

**Status**: ✅ Implemented
**Last Updated**: 2025-11-17

## Overview
Critical architecture pattern for efficient memory usage with multiple concurrent transcription pipelines. Single shared Whisper model with multiple lightweight contexts prevents memory exhaustion.

## Problem Statement

### Memory Exhaustion Issue
**Discovered**: 2025-11-17

Each pipeline was creating its own Whisper model (1.6GB each) instead of sharing a single model across contexts. This caused massive memory usage:
- Single client: ~2GB (acceptable)
- Two clients: ~4GB (wasteful)
- Multiple clients: 14-15GB+ (unsustainable)

### Root Cause
Pipeline initialization was loading the entire model from disk for each connection:
```go
// WRONG - loads 1.6GB model per pipeline
pipeline := transcription.NewPipeline(PipelineConfig{
    ModelPath: "/path/to/model.bin",  // Each pipeline loads from disk
    // ...
})
```

## Solution Architecture

### One Model, Many Contexts
Whisper.cpp is designed for this pattern:
- **Model**: Weights and parameters (1.6GB) - shared across all pipelines
- **Context**: Processing state (few MB) - one per pipeline/session

### Implementation Pattern

#### 1. Load Model Once at Startup
```go
// server/cmd/server/main.go
sharedModel, err := whisper.New(cfg.Transcription.ModelPath)
if err != nil {
    log.Fatal("Failed to load Whisper model: %v", err)
}
defer sharedModel.Close()
log.Info("Whisper model loaded: %s", cfg.Transcription.ModelPath)
```

#### 2. Pass Model to WebRTC Manager
```go
// Create WebRTC manager with shared model
webrtcManager := webrtc.NewManager(apiServer, sharedModel, cfg, log)
```

#### 3. Manager Passes to Pipelines
```go
// server/internal/webrtc/manager.go
pipeline, err := transcription.NewPipeline(transcription.PipelineConfig{
    SharedWhisperModel: m.sharedWhisperModel,  // Pass model, not path
    // ...
})
```

#### 4. Pipeline Creates Context from Model
```go
// server/internal/transcription/whisper.go
type WhisperTranscriberShared struct {
    model   whisper.Model     // Shared model
    context whisper.Context   // Unique context per transcriber
}

func NewWhisperTranscriberShared(model whisper.Model) (*WhisperTranscriberShared, error) {
    ctx := model.NewContext()  // Lightweight context from shared model
    return &WhisperTranscriberShared{
        model:   model,
        context: ctx,
    }, nil
}
```

### API Changes

#### WhisperTranscriberShared
**New constructor**: `NewWhisperTranscriberShared(model whisper.Model)`
- Takes shared model instead of path
- Creates context from model
- Each instance has unique context
- All share same model in memory

#### PipelineConfig
**New field**: `SharedWhisperModel whisper.Model`
- Optional field (backward compatible)
- Takes precedence over `ModelPath` if provided
- Enables shared model pattern

## Memory Benefits

### Before (Multiple Models)
```
Client 1: 1.6GB model + 5MB context = 1.605GB
Client 2: 1.6GB model + 5MB context = 1.605GB
Client 3: 1.6GB model + 5MB context = 1.605GB
Total: ~4.8GB
```

### After (Shared Model)
```
Shared: 1.6GB model
Client 1: 5MB context
Client 2: 5MB context
Client 3: 5MB context
Total: ~1.615GB (70% reduction!)
```

### Scalability Impact
- 10 concurrent clients: **19GB → 1.65GB** (91% reduction)
- 100 concurrent clients: **190GB → 2.1GB** (98% reduction)

## Lifecycle Management

### Model Lifetime
- **Created**: Server startup
- **Lives**: Entire server lifetime
- **Closed**: Server shutdown
- **Never recreated**: Per-connection

### Context Lifetime
- **Created**: Pipeline initialization (per connection)
- **Lives**: Duration of connection
- **Closed**: Pipeline cleanup
- **Recreated**: Each new connection

## Thread Safety

### Whisper.cpp Guarantees
- Model is immutable after loading (thread-safe for reads)
- Each context is independent (no shared state)
- Concurrent transcription is safe with separate contexts

### Implementation Safety
- Model loaded once in main goroutine
- Passed by reference (not copied)
- Each pipeline gets independent context
- No mutex needed (contexts don't share state)

## Backward Compatibility

### Old Code Still Works
```go
// Old way (still supported) - creates own model
pipeline := transcription.NewPipeline(PipelineConfig{
    ModelPath: "/path/to/model.bin",
    // ...
})
```

### New Code (Recommended)
```go
// New way - uses shared model
pipeline := transcription.NewPipeline(PipelineConfig{
    SharedWhisperModel: sharedModel,
    // ...
})
```

### Migration Path
1. Load shared model at startup
2. Pass to WebRTC manager
3. Manager passes to pipelines
4. Old `ModelPath` can be removed from config

## Design Decisions

### Why Load at Startup (Not Lazy)?
- **Fail fast**: Model loading errors caught immediately
- **Predictable startup**: No surprise delays on first connection
- **Clear ownership**: Main function owns model lifecycle
- **Simple cleanup**: Single defer statement

### Why Not Pool Contexts?
- **Contexts are lightweight**: ~5MB each (negligible overhead)
- **Simple lifecycle**: Create on connect, close on disconnect
- **No complexity**: Pooling adds coordination overhead
- **Adequate performance**: Context creation is fast (<100ms)

### Why Not Singleton Pattern?
- **Explicit is better**: Main function clearly owns model
- **Testable**: Can inject different models for testing
- **Flexible**: Could support multiple models in future
- **No hidden state**: All dependencies visible

## Performance Characteristics

### Model Loading
- **Time**: ~2-3 seconds (one-time cost)
- **Memory**: 1.6GB (constant regardless of clients)
- **Disk I/O**: ~1.6GB read (once at startup)

### Context Creation
- **Time**: ~50-100ms per pipeline
- **Memory**: ~5MB per pipeline
- **Incremental**: Only when client connects

### Transcription Performance
- **Unchanged**: Context performance identical to model-per-pipeline
- **Concurrency**: Multiple contexts transcribe in parallel
- **No contention**: Each context independent

## Known Limitations

1. **Single model per server**: All clients use same Whisper model
2. **No model hot-swap**: Changing model requires server restart
3. **Fixed at startup**: Model path set in config, loaded once

## Future Enhancements

### Multiple Models
Support different models for different clients:
```go
// Load multiple models
largeModel := whisper.New("large-v3-turbo.bin")
smallModel := whisper.New("base.bin")

// Select based on client needs
if client.NeedsHighAccuracy {
    pipeline.UseModel(largeModel)
} else {
    pipeline.UseModel(smallModel)
}
```

### Model Hot-Reload
Reload model without server restart:
```go
// Graceful model swap
newModel := whisper.New(newPath)
atomic.StorePointer(&sharedModel, newModel)
oldModel.Close()
```

### Model Metrics
Track model usage and performance:
```go
type ModelMetrics struct {
    ActiveContexts    int
    TotalTranscriptions int
    AverageLatency    time.Duration
}
```

## Related Systems
- [Per-Client Pipeline Architecture](per-client-pipeline.md) - How pipelines are created per connection
- [Whisper and RNNoise Setup](../dependencies/whisper-and-rnnoise-setup.md) - Model installation
- [Transcription Pipeline](transcription-pipeline.md) - Pipeline implementation details

## Files Modified
- `server/cmd/server/main.go` - Model loading at startup
- `server/internal/webrtc/manager.go` - Model storage and passing
- `server/internal/transcription/whisper.go` - WhisperTranscriberShared implementation
- `server/internal/transcription/pipeline.go` - SharedWhisperModel config field
