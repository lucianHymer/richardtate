# Memory Leak Fix

## The Problem
Server was using 14-15GB of memory, eventually hitting 35GB+ and crashing the system.

## The Root Cause
**Each client connection was loading its own copy of the 1.6GB Whisper model!**

```go
// WRONG - What we were doing:
func NewPipeline() {
    model := whisper.New("model.bin")  // Loads 1.6GB PER CONNECTION!
    ctx := model.NewContext()
}
```

With 10 connections, that's 16GB just for duplicate models.

## The Fix
Load the Whisper model ONCE at server startup and share it across all connections:

```go
// CORRECT - What we do now:
// At startup:
sharedModel := whisper.New("model.bin")  // Load ONCE

// Per connection:
func NewPipeline(sharedModel) {
    ctx := sharedModel.NewContext()  // Lightweight context
}
```

## Implementation
1. `server/internal/transcription/whisper_shared.go` - Shared model wrapper
2. `server/cmd/server/main.go` - Loads model once at startup
3. `server/internal/webrtc/manager.go` - Passes shared model to pipelines
4. `server/internal/transcription/pipeline.go` - Uses shared model

## Expected Memory Usage
- **Before**: 1.6GB × N connections (14-15GB with ~10 connections)
- **After**: 1.6GB base + ~100MB per connection (2-3GB total)

## Why This Happened
Misunderstood the Whisper.cpp architecture:
- `whisper.Model` = The weights (1.6GB, should be shared)
- `whisper.Context` = Processing state (small, one per session)

We were treating models like contexts, creating a new one for each connection.