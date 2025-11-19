# Parakeet Streaming Integration Guide

## Current State (2024-11-18)
This is a work-in-progress implementation of Parakeet streaming. The core streaming logic is implemented but needs cleanup and integration.

## ⚠️ IMPORTANT: Cleanup Needed

### Files Created (Need Integration)
- `scripts/parakeet_worker_streaming.py` - NEW streaming Python worker ✅
- `server/internal/transcription/parakeet_shared.go` - REPLACED with streaming version (was batch) ⚠️
- `server/internal/transcription/parakeet_transcriber.go` - Still exists, needs update for streaming

### Duplicate/Conflicting Code
The file `parakeet_shared.go` now contains `SharedParakeetWorkerStreaming` but references throughout the codebase still expect `SharedParakeetWorker`. Need to either:
1. Rename the streaming type back to `SharedParakeetWorker` (recommended)
2. Update all references to use the new type name

### Integration Status
- ❌ NOT integrated with main.go yet
- ❌ NOT integrated with pipeline.go yet
- ❌ Build currently BROKEN due to duplicate types
- ✅ Core streaming logic implemented
- ✅ Python worker protocol complete

## Overview
This implementation replaces the batch-based Parakeet approach with real-time streaming transcription using the `transcribe_stream()` API from parakeet-mlx.

## Key Components Created

### 1. Python Worker (`scripts/parakeet_worker_streaming.py`)
- Maintains streaming contexts per client
- Protocol: `start_stream`, `add_audio`, `end_stream` commands
- Returns incremental transcription results

### 2. Go Worker (`parakeet_shared_streaming.go`)
- `SharedParakeetWorkerStreaming` - Manages subprocess and client sessions
- Each pipeline gets unique client ID
- Handles 1-second buffering internally (no VAD needed)

### 3. Transcriber Adapter (`parakeet_transcriber_streaming.go`)
- `ParakeetTranscriberStreaming` - Implements ASRTranscriber interface
- Simple wrapper around streaming worker
- Manages client lifecycle

## Integration Changes Required

### 1. Update `server/cmd/server/main.go`

Replace the Parakeet initialization:

```go
// OLD:
var sharedParakeetWorker *transcription.SharedParakeetWorker

// NEW:
var sharedParakeetWorker *transcription.SharedParakeetWorkerStreaming

// OLD:
sharedParakeetWorker, err = transcription.NewSharedParakeetWorker(...)

// NEW:
sharedParakeetWorker, err = transcription.NewSharedParakeetWorkerStreaming(...)
```

### 2. Update `server/internal/webrtc/manager.go`

Update the ManagerConfig type:

```go
type ManagerConfig struct {
    // OLD:
    SharedParakeetWorker *transcription.SharedParakeetWorker

    // NEW:
    SharedParakeetWorker *transcription.SharedParakeetWorkerStreaming
}
```

### 3. Update `server/internal/transcription/asr_factory.go`

Update the factory to create streaming transcriber:

```go
case "parakeet":
    if config.SharedParakeetWorker == nil {
        return nil, fmt.Errorf("SharedParakeetWorker is required for parakeet engine")
    }
    // NEW: Use streaming transcriber
    return NewParakeetTranscriberStreaming(config.SharedParakeetWorker)
```

### 4. Update `server/internal/transcription/pipeline.go`

Modify pipeline to skip VAD/chunker for Parakeet:

```go
func (p *TranscriptionPipeline) ProcessChunk(pcmData []int16) error {
    // Process through RNNoise
    denoised := p.rnnoise.Process(pcmData)

    // NEW: Check if using Parakeet streaming
    if p.engine == "parakeet" {
        // Direct to transcriber (handles buffering internally)
        text, err := p.asr.Transcribe(denoised)
        if err != nil {
            // Handle error
        } else if text != "" {
            // Send result
        }
        return nil
    }

    // Existing Whisper path with VAD/chunker
    p.chunker.ProcessSamples(denoised)
    return nil
}
```

### 5. Update Pipeline Config Types

Update ASRConfig in various places:

```go
type ASRConfig struct {
    // Update type:
    SharedParakeetWorker *SharedParakeetWorkerStreaming
}

type PipelineConfig struct {
    // Update type:
    SharedParakeetWorker *SharedParakeetWorkerStreaming
}
```

## Configuration Changes

### Client Config (`client/config.yaml`)
```yaml
transcription:
  vad:  # Note: Only used with Whisper engine, ignored for Parakeet
    energy_threshold: 500
    # ... other VAD settings
```

Document that VAD settings are ignored when using Parakeet.

### Server Config (`server/config.yaml`)
No changes needed - Parakeet still uses same config:

```yaml
transcription:
  engine: "parakeet"  # Enables streaming mode
  parakeet:
    model_id: "mlx-community/parakeet-tdt-0.6b-v3"
    script_path: "/path/to/scripts/parakeet_worker_streaming.py"  # Use streaming script
    python_path: "python3"
```

## Benefits of Streaming

1. **Natural Speech Flow**: No artificial pauses needed
2. **Real-time Feedback**: See words as you speak them
3. **No VAD Calibration**: Eliminates threshold tuning complexity
4. **Better UX**: Feels like live stenographer

## Technical Details

### Buffering Strategy
- Simple time-based: Accumulate 1 second (5 × 200ms chunks)
- No VAD logic needed
- Buffering happens in Go (SharedParakeetWorkerStreaming)
- Python maintains streaming context state

### Streaming Protocol
```json
// Start stream
{"command": "start_stream", "client_id": "uuid", "context_size": [256, 256]}

// Add audio (1-second chunks)
{"command": "add_audio", "client_id": "uuid", "audio": "base64...", "sample_rate": 16000}

// Response
{"text": "transcribed text", "is_final": false, "client_id": "uuid"}

// End stream
{"command": "end_stream", "client_id": "uuid"}
```

### Client Lifecycle
1. Pipeline creates transcriber → Gets unique client ID
2. Audio chunks arrive → Buffer until 1 second
3. Send to streaming context → Get incremental results
4. Pipeline closes → End streaming session

## Testing

### Manual Test
1. Set `engine: "parakeet"` in server config
2. Start server with streaming worker
3. Start client and begin recording
4. Speak continuously without long pauses
5. Observe real-time transcription output

### Expected Behavior
- Text appears within 1-2 seconds of speaking
- No need for silence to trigger transcription
- Continuous speech flows naturally
- Short utterances still captured

## Migration Path

1. **Phase 1**: Test streaming implementation alongside existing
2. **Phase 2**: Replace SharedParakeetWorker with streaming version
3. **Phase 3**: Remove old batch-based Parakeet code
4. **Phase 4**: Update documentation for streaming mode

## Troubleshooting

### No Output
- Check Python worker is using `parakeet_worker_streaming.py`
- Verify model loads successfully (check stderr logs)
- Ensure audio is reaching the worker (debug logs)

### Delayed Output
- Normal: 1-second buffering before first output
- Check if model is running on GPU (MLX acceleration)

### Python Errors
- Verify `parakeet-mlx` supports `transcribe_stream()` method
- Check Python environment has all dependencies
- Review stderr output for import errors

## Next Steps for Implementation Team

### Quick Path (Recommended)
1. Rename `SharedParakeetWorkerStreaming` to `SharedParakeetWorker` in `parakeet_shared.go`
2. Update `parakeet_transcriber.go` to use the streaming worker's client-based approach
3. Update `pipeline.go` to skip VAD/chunker when `engine == "parakeet"`
4. Update script path in config to use `parakeet_worker_streaming.py`
5. Test with real model

### Key Design Decisions Made
- **1-second buffering** in Go (not Python) - simpler state management
- **Per-client streaming contexts** - each pipeline gets unique client ID
- **No VAD for Parakeet** - just time-based chunking (5 × 200ms = 1 second)
- **Shared worker pattern maintained** - one subprocess, multiple clients

### Why This Is Better
- Natural speech flow without managing silence pauses
- Real-time feedback (see words as you speak)
- No VAD calibration needed
- Fixes the problem where users have to pause unnaturally between sentences