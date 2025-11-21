# Architecture Documentation Index

Complete exploration of the Hammerspoon + Client Daemon system for voice transcription.

## Quick Start

**New to the architecture?** Start here:
1. Read [ARCHITECTURE_EXPLORATION_SUMMARY.txt](ARCHITECTURE_EXPLORATION_SUMMARY.txt) (5 min read)
2. Review the architecture diagram in [HAMMERSPOON_ARCHITECTURE.md](HAMMERSPOON_ARCHITECTURE.md#architecture-diagram)
3. Study the detailed data flows in [HAMMERSPOON_ARCHITECTURE.md](HAMMERSPOON_ARCHITECTURE.md#detailed-data-flow)

## Documents

### Primary Architecture Documentation

#### [HAMMERSPOON_ARCHITECTURE.md](HAMMERSPOON_ARCHITECTURE.md) - 27KB, Comprehensive
**The authoritative guide to the entire system.**

Contents:
- High-level architecture diagram
- Three-tier system overview
- Detailed data flow (recording session)
- Detailed data flow (calibration session)
- Component-by-component breakdown:
  - Hammerspoon (init.lua - 215 lines)
  - Hammerspoon calibration (calibration.lua - 490 lines)
  - Client daemon (main.go - 310 lines)
  - API server (server.go - 549 lines)
  - WebRTC client (client.go - 600+ lines)
- Message format specifications
- Key design decisions and rationales
- Threading model and concurrency patterns
- Performance characteristics
- Future enhancement opportunities
- Summary and architectural implications

**Use this when:**
- Planning a refactor to remove Hammerspoon
- Understanding the complete data flow
- Making architectural decisions
- Documenting for code review
- Onboarding new developers

#### [ARCHITECTURE_EXPLORATION_SUMMARY.txt](ARCHITECTURE_EXPLORATION_SUMMARY.txt) - 3.5KB, Quick Reference
**Condensed summary of the exploration findings.**

Contents:
- Three-tier architecture overview
- Hammerspoon and Client Daemon responsibilities
- Communication patterns (HTTP, WebSocket, WebRTC)
- Step-by-step flows for recording and calibration
- Files explored and line counts
- Key design patterns
- Critical components and endpoints
- Threading and concurrency summary
- Performance characteristics
- Future enhancement opportunities
- Key insights

**Use this when:**
- You need a quick overview
- Explaining the system to others
- Quick reference during implementation
- Understanding the scope of a feature change

## System Architecture

```
┌─────────────────────────────────┐
│      Hammerspoon (macOS UI)     │  ← User hotkeys, text insertion
│      - init.lua (215 lines)     │     Canvas indicators
│      - calibration.lua (490)    │     Calibration wizard
└────────────┬────────────────────┘
             │ HTTP + WebSocket
             │ (localhost:8081)
┌────────────▼────────────────────┐
│    Client Daemon (Go)            │  ← Audio capture, WebRTC, sessions
│    - main.go (310 lines)         │     API endpoints, broadcasting
│    - api/server.go (549 lines)   │     Calibration logic
│    - webrtc/client.go (600+)     │     Connection management
└────────────┬────────────────────┘
             │ WebRTC
             │ (audio + control)
┌────────────▼────────────────────┐
│  Transcription Server (Go)       │  ← Whisper/Parakeet ASR
│  - WebRTC signaling              │     RNNoise, VAD, chunking
│  - Audio processing              │     Transcription streaming
└─────────────────────────────────┘
```

## Key Findings

### Separation of Concerns
- **Hammerspoon**: All UI/UX (hotkeys, indicators, text insertion, calibration wizard)
- **Client Daemon**: Business logic (audio capture, WebRTC, sessions, calibration endpoints)
- **Server**: Transcription (Whisper/Parakeet, RNNoise, VAD, chunking)

Benefit: Can replace Hammerspoon with web UI, Electron, iOS app, etc. without changing daemon/server.

### Communication Patterns
- **HTTP (Synchronous)**: Control commands (start, stop, calibration)
- **WebSocket (Asynchronous)**: Streaming transcription chunks to Hammerspoon
- **WebRTC DataChannel**: Audio and transcription messages to/from server

### Recording Flow (Simplified)
```
User presses Ctrl+N
  → Hammerspoon HTTP POST /start
    → Client daemon starts audio capture
      → Audio sent to server via WebRTC
        → Server transcribes
          → Results sent back via WebRTC
            → Client broadcasts via WebSocket
              → Hammerspoon inserts text
User presses Ctrl+N again
  → Hammerspoon HTTP POST /stop
    → Client daemon logs session
```

### Calibration Flow (Simplified)
```
User presses Ctrl+Alt+C
  → Hammerspoon opens 3-step wizard
    → Step 1: Record background (HTTP POST /api/calibrate/record)
      → Daemon captures audio, sends to server for analysis
        → Server returns energy stats
          → Hammerspoon displays stats, proceeds to Step 2
    → Step 2: Record speech (same as Step 1)
    → Step 3: Display results and allow save
      → User clicks "Save & Close"
        → Hammerspoon HTTP POST /api/calibrate/save
          → Daemon updates config file
          → Daemon reloads config in-memory (NO RESTART NEEDED!)
```

## Component Responsibilities

### Hammerspoon (Lua)

**init.lua (215 lines)**
- Hotkey bindings: Ctrl+N (record toggle), Ctrl+Alt+C (calibration)
- HTTP client: POST /start, POST /stop
- WebSocket client: Receives {"chunk": "text", "final": boolean}
- Text insertion: `hs.eventtap.keyStrokes(text)`
- Visual indicator: Canvas floating window (top-right, red dot + "Recording...")

**calibration.lua (490 lines)**
- 3-step wizard UI (all canvas-based, no WebView)
- Step 1: Background recording UI + button
- Step 2: Speech recording UI + button
- Step 3: Results display with bars + Save/Cancel buttons
- Progress tracking: Timer updates UI every 0.5 seconds
- HTTP requests: /api/calibrate/record, /calculate, /save

### Client Daemon (Go)

**main.go (310 lines)**
- Entry point: Parse flags, load config, setup logging
- WebRTC client: Connect to server, handle audio sending
- Audio capture: Microphone input, send chunks via WebRTC
- Session state: Track sessionChunks, sessionStart, sessionRecording
- Message handler: Receive transcriptions, broadcast to WebSocket
- API server: Start HTTP server on localhost:8081
- Lifecycle: Wait for interrupt, graceful shutdown

**api/server.go (549 lines)**
- HTTP endpoints:
  - `/start`: Initialize session, start audio capture
  - `/stop`: Stop audio, log session
  - `/transcriptions`: WebSocket upgrade for streaming
  - `/api/calibrate/record`: Record audio, return stats
  - `/api/calibrate/calculate`: Calculate threshold
  - `/api/calibrate/save`: Save config, reload in-memory
- WebSocket: Manage clients map, broadcast transcriptions
- Audio analysis: Send audio to server /api/v1/analyze-audio
- Config management: Update YAML, reload in-place

**webrtc/client.go (600+ lines)**
- Connection: WebSocket signaling + DataChannel for audio
- Audio sending: SendAudioChunk() serializes and sends
- Message reception: handleMessage() dispatches to callback
- Reconnection: Exponential backoff (1s, 2s, 4s, 8s, 16s, 30s)
- Buffering: Auto-buffers chunks during reconnection
- Control: SendControlStart/Stop for session signaling

## Message Formats

### Hammerspoon ↔ Client Daemon (HTTP + WebSocket)

**HTTP Control** (synchronous):
```
POST /start       → Start recording
POST /stop        → Stop recording
POST /api/calibrate/record → Record audio
POST /api/calibrate/calculate → Calculate threshold
POST /api/calibrate/save → Save config
```

**WebSocket Streaming** (asynchronous):
```json
{
  "chunk": "Transcribed text here",
  "final": true
}
```

### Client Daemon ↔ Server (WebRTC DataChannel)

**Audio chunk** (Client → Server):
```go
type AudioChunkData struct {
    SampleRate int
    Channels   int
    Data       []byte  // PCM int16
    SequenceID uint64
}
```

**Transcription** (Server → Client):
```go
type TranscriptData struct {
    Text string
}
```

## Critical Design Patterns

1. **HTTP + WebSocket Hybrid**
   - HTTP for synchronous control (simple request-response)
   - WebSocket for asynchronous streaming (efficient server-push)

2. **In-Place Config Reload**
   - After calibration, `cfg.Reload()` updates Config struct
   - All references see new values immediately
   - No daemon restart required

3. **Session Tracking in Daemon**
   - `sessionChunks` array accumulates transcription text
   - Logged to debug log on session stop
   - Enables recovery if Hammerspoon crashes

4. **Canvas-Based UI**
   - Hammerspoon uses canvas drawing (not WebView)
   - Simpler, faster, native-feeling
   - No HTML/CSS/JS dependencies

5. **Broadcasting to WebSocket Clients**
   - API server maintains `wsClients` map
   - `BroadcastTranscription()` sends to all connected clients
   - Supports multiple UIs (theoretically)

## Performance

**Latency (end-to-end)**:
- User presses hotkey → recording starts: < 100ms
- Speech → transcription appears: 1-3 seconds (dominated by model)
- Total user-perceived latency: 1-3 seconds

**Memory**:
- Hammerspoon: ~50MB
- Client daemon: ~100MB
- WebRTC connection: ~50MB
- Session buffer: ~1MB

**CPU**:
- Idle: < 1%
- Recording: 5-10%
- Transcription: Handled by server

## Threading

**Goroutines**:
1. Main: Initialization and cleanup
2. Audio capture: Read microphone, send chunks
3. WebRTC signaling: Offer/answer/ICE candidates
4. API server: HTTP request handling
5. WebSocket readers: One per connected client
6. Reconnection: Exponential backoff loop

**Synchronization**:
- `sessionMu`: Protects sessionChunks, sessionStart, sessionRecording
- `wsClientsMu`: Protects WebSocket clients map
- `connectedMu`: Protects connection state
- `reconnectingMu`: Protects reconnection state

## Endpoints Reference

### Hammerspoon → Client Daemon (HTTP on localhost:8081)

| Endpoint | Method | Purpose | Request | Response |
|----------|--------|---------|---------|----------|
| `/health` | GET | Health check | — | `{"status": "ok"}` |
| `/start` | POST | Start recording | — | `{"status": "started"}` |
| `/stop` | POST | Stop recording | — | `{"status": "stopped"}` |
| `/status` | GET | Get status | — | `{"running": boolean}` |
| `/transcriptions` | GET | WebSocket upgrade | — | WebSocket connection |
| `/api/calibrate/record` | POST | Record audio | `{"duration_seconds": 5}` | `{min, max, avg, p5, p95}` |
| `/api/calibrate/calculate` | POST | Calculate threshold | `{background: {...}, speech: {...}}` | `{"threshold": float}` |
| `/api/calibrate/save` | POST | Save config | `{"threshold": float}` | `{"success": bool}` |

## Future Enhancement Opportunities

### V2 Features
- Preview mode (show transcription before inserting)
- Processing modes (casual, professional, technical)
- Multi-language support
- Cross-platform UI (replace Hammerspoon with web, Electron, iOS)
- Offline mode (local transcription fallback)

### Architecture Extensions
- Streaming UI (WebSocket for real-time energy feedback)
- Insertion persistence (save text if insertion fails)
- Server-side session tracking
- Load balancing (multiple servers)
- Model hot-swap (change models without restart)

## Use Cases

### Planning Hammerspoon Removal
1. Read this index to understand current architecture
2. Study HAMMERSPOON_ARCHITECTURE.md for detailed data flows
3. Identify which Hammerspoon features need to be replicated
4. Design alternative UI using HTTP/WebSocket APIs (all unchanged)

### Implementing Alternative UI
1. Keep client daemon and server unchanged
2. Implement new UI using HTTP/WebSocket APIs
3. Required endpoints:
   - POST /start
   - POST /stop
   - GET /transcriptions (WebSocket)
   - POST /api/calibrate/* (for calibration wizard)
4. No changes to core transcription system

### Debugging Data Flow
1. Enable debug logging in config
2. Monitor HTTP requests to localhost:8081
3. Monitor WebSocket messages
4. Monitor WebRTC DataChannel via server logs
5. Check debug log file for session recovery

### Onboarding New Developers
1. Read ARCHITECTURE_EXPLORATION_SUMMARY.txt
2. Study HAMMERSPOON_ARCHITECTURE.md for your area
3. Look at specific component code (line ranges provided)
4. Run system while monitoring logs and endpoints

## File Locations

**Hammerspoon**:
- `hammerspoon/init.lua` - Main recording control
- `hammerspoon/calibration.lua` - Calibration wizard
- `hammerspoon/README.md` - User documentation

**Client Daemon**:
- `client/cmd/client/main.go` - Entry point
- `client/internal/api/server.go` - HTTP endpoints
- `client/internal/webrtc/client.go` - WebRTC connection
- `client/internal/audio/capture.go` - Audio capture
- `client/internal/config/config.go` - Configuration

**Documentation**:
- `docs/HAMMERSPOON_ARCHITECTURE.md` - Comprehensive guide (this directory)
- `docs/ARCHITECTURE_EXPLORATION_SUMMARY.txt` - Quick reference
- `docs/DAEMON-SETUP.md` - Setup instructions
- `.claude/knowledge/` - Additional architecture notes

## Summary

The Hammerspoon + Client Daemon architecture successfully separates concerns:
- **Hammerspoon** handles all UI/UX concerns (hotkeys, indicators, text insertion)
- **Client Daemon** handles business logic (audio capture, WebRTC, sessions)
- **Server** handles transcription (Whisper/Parakeet, RNNoise, VAD)

This clean separation makes it straightforward to replace Hammerspoon with alternative UIs while keeping the core system unchanged. The architecture is well-designed for extensibility and maintainability.

---

**Documentation created**: 2025-11-19
**Status**: Complete exploration and documentation
