# Hammerspoon + Client Daemon - Quick Reference Card

## System Architecture

```
┌─────────────────────────┐
│ Hammerspoon (Lua)       │  Port: localhost:8081 (HTTP)
│ - init.lua              │  - Hotkeys: Ctrl+N, Ctrl+Alt+C
│ - calibration.lua       │  - WebSocket: /transcriptions
└──────────┬──────────────┘
           │ HTTP + WebSocket
┌──────────▼──────────────┐
│ Client Daemon (Go)      │
│ - main.go               │  Entry point: localhost:8081
│ - api/server.go         │  Connects to: server:port/api/v1/stream/signal
│ - webrtc/client.go      │
└──────────┬──────────────┘
           │ WebRTC
┌──────────▼──────────────┐
│ Transcription Server    │
└─────────────────────────┘
```

## Hotkeys

| Hotkey | Action |
|--------|--------|
| `Ctrl+N` | Toggle recording (start/stop) |
| `Ctrl+Alt+C` | Open calibration wizard |
| `Cmd+Alt+Ctrl+R` | Reload Hammerspoon |

## HTTP Endpoints (localhost:8081)

### Recording Control

| Method | Endpoint | Purpose | Response |
|--------|----------|---------|----------|
| POST | `/start` | Start recording | `{"status": "started"}` |
| POST | `/stop` | Stop recording | `{"status": "stopped"}` |
| GET | `/status` | Get recording status | `{"running": boolean}` |
| GET | `/health` | Health check | `{"status": "ok"}` |

### Calibration

| Method | Endpoint | Purpose | Request | Response |
|--------|----------|---------|---------|----------|
| POST | `/api/calibrate/record` | Record audio | `{"duration_seconds": 5}` | `{min, max, avg, p5, p95}` |
| POST | `/api/calibrate/calculate` | Calculate threshold | `{background: {...}, speech: {...}}` | `{"threshold": float}` |
| POST | `/api/calibrate/save` | Save to config | `{"threshold": float}` | `{"success": bool}` |

### Streaming

| Method | Endpoint | Purpose |
|--------|----------|---------|
| GET | `/transcriptions` | WebSocket upgrade |

## WebSocket Messages

### Hammerspoon ← Client Daemon

```json
{
  "chunk": "Transcribed text here",
  "final": true
}
```

**Events**: `received`, `open`, `closed`, `fail`

## WebRTC DataChannel Messages

### Client → Server

**Audio chunk**:
```go
type AudioChunkData struct {
    SampleRate int
    Channels   int
    Data       []byte  // PCM int16
    SequenceID uint64
}
// Message type: MessageTypeAudioChunk
```

**Control**:
- `MessageTypeControlStart` - Begin recording (includes VAD settings)
- `MessageTypeControlStop` - End recording

### Server → Client

**Transcription**:
```go
type TranscriptData struct {
    Text string
}
// Message type: MessageTypeTranscriptFinal or MessageTypeTranscriptPartial
```

## Data Flow Summary

### Recording (User presses Ctrl+N)

1. Hammerspoon hotkey → `toggleRecording()`
2. HTTP POST `/start`
3. Client: `onStart()` handler
   - Initialize `sessionChunks = []`
   - Record `sessionStart` time
   - Send `MessageTypeControlStart` via WebRTC
   - Call `capturer.Start()`
4. Audio capture loop
   - Read 200ms chunks at 16kHz
   - Send via WebRTC `MessageTypeAudioChunk`
5. Server processes, transcribes
6. Server sends `MessageTypeTranscriptFinal`
7. Client receives, broadcasts via WebSocket
8. Hammerspoon receives, calls `hs.eventtap.keyStrokes(text)`
9. User presses Ctrl+N again
10. HTTP POST `/stop`
11. Client: `onStop()` handler
    - Stop audio capture
    - Log complete session
    - Send `MessageTypeControlStop`

### Calibration (User presses Ctrl+Alt+C)

1. Hammerspoon hotkey → `calibration.show()`
2. Display Step 1 UI (Background Recording)
3. User clicks "Start Recording"
4. HTTP POST `/api/calibrate/record` (5 seconds)
5. Client records, sends to server `/api/v1/analyze-audio`
6. Server returns energy stats
7. Hammerspoon displays Step 2 UI (Speech Recording)
8. Repeat steps 4-6 for speech
9. HTTP POST `/api/calibrate/calculate`
10. Client calculates: `threshold = background_P95 * 1.5`
11. Hammerspoon displays Step 3 UI (Results + Save)
12. User clicks "Save & Close"
13. HTTP POST `/api/calibrate/save`
14. Client: `config.UpdateVADThreshold()` + `cfg.Reload()`
15. Config updated on disk AND in-memory (no restart!)

## Key Components

### Hammerspoon (init.lua - 215 lines)

- **Hotkeys**: `hs.hotkey.bind()` for Ctrl+N and Ctrl+Alt+C
- **HTTP**: `hs.http.doAsyncRequest()` for control
- **WebSocket**: `hs.websocket.new()` for streaming
- **Text insertion**: `hs.eventtap.keyStrokes(text)`
- **UI**: Canvas-based floating indicator (200x40px, top-right)

### Hammerspoon (calibration.lua - 490 lines)

- **UI**: Canvas-based 3-step wizard (500x400px)
- **Recording**: Progress tracking with timer (0.5s updates)
- **HTTP**: Async requests to calibration endpoints
- **State**: Step tracking, stats storage, threshold saving

### Client Daemon (main.go - 310 lines)

- **Flags**: `--config`, `--calibrate`, `--yes`
- **WebRTC**: Connect to `server_url/api/v1/stream/signal`
- **Audio**: Capture from device, send via WebRTC
- **Session**: Track `sessionChunks`, `sessionStart`, `sessionRecording`
- **API**: Start HTTP server on `api_bind_address` (default localhost:8081)
- **Lifecycle**: Wait for interrupt, graceful shutdown

### Client Daemon (api/server.go - 549 lines)

- **HTTP**: 8 endpoints for control and calibration
- **WebSocket**: Connection management, broadcasting
- **Calibration**: Record → Analyze → Calculate → Save
- **Config**: Update YAML, hot-reload without restart
- **Audio**: Capture for calibration, send to server for analysis

### Client Daemon (webrtc/client.go - 600+ lines)

- **Connection**: WebSocket signaling + DataChannel
- **Audio**: `SendAudioChunk()` serializes and sends
- **Messages**: `handleMessage()` dispatches to callback
- **Reconnection**: Exponential backoff (1s, 2s, 4s, 8s, 16s, 30s max)
- **Buffering**: Auto-buffers chunks during disconnection

## Configuration

### Server URL

```yaml
server:
  url: "ws://localhost:8080"  # Or ws:// for signaling, http:// for REST
```

### Client Daemon Binding

```yaml
client:
  api_bind_address: "localhost:8081"  # HTTP server for Hammerspoon
```

### Audio Device

```yaml
audio:
  device_name: ""  # Empty = default device
```

### VAD Settings (sent in control.start)

```yaml
transcription:
  vad:
    energy_threshold: 184.2
    silence_threshold_ms: 1000
    min_chunk_duration_ms: 500
    max_chunk_duration_ms: 30000
    speech_density_threshold: 0.6
```

## Performance

| Metric | Value |
|--------|-------|
| Hotkey → Recording starts | < 100ms |
| Speech → Text appears | 1-3s |
| Hammerspoon memory | ~50MB |
| Client daemon memory | ~100MB |
| CPU (idle) | < 1% |
| CPU (recording) | 5-10% |
| WebSocket latency | < 10ms |

## Threading

**Goroutines**:
1. Main: Initialization and cleanup
2. Audio capture: Microphone input loop
3. WebRTC signaling: Offer/answer/ICE
4. API server: HTTP request handling
5. WebSocket readers: One per client
6. Reconnection: Exponential backoff

**Mutexes**:
- `sessionMu`: sessionChunks, sessionStart, sessionRecording
- `wsClientsMu`: WebSocket clients map
- `connectedMu`: Connection state
- `reconnectingMu`: Reconnection state

## Message Examples

### HTTP: Start Recording

```bash
curl -X POST http://localhost:8081/start
# Response: {"status": "started"}
```

### HTTP: Record Calibration Audio

```bash
curl -X POST http://localhost:8081/api/calibrate/record \
  -H "Content-Type: application/json" \
  -d '{"duration_seconds": 5}'
# Response: {"min": 12.3, "max": 89.4, "avg": 45.2, "p5": 34.5, "p95": 78.1}
```

### WebSocket: Receive Transcription

```
Connected to ws://localhost:8081/transcriptions
Received: {"chunk": "Hello world", "final": true}
```

## Debugging Tips

1. **Enable debug logging**: Set `debug: true` in config
2. **Monitor HTTP**: Check logs for POST /start, /stop, /api/calibrate/*
3. **Monitor WebSocket**: Watch for chunk messages
4. **Check session log**: `tail -f ~/.config/richardtate/debug.log`
5. **Verify connection**: `curl http://localhost:8081/health`

## Future Enhancement Points

- Preview mode (show transcription before inserting)
- Processing modes (casual, professional, technical)
- Alternative UIs (web, Electron, iOS) - same HTTP/WebSocket APIs
- Offline mode (local transcription fallback)
- Multiple server support (load balancing)

---

**Quick Reference Card** - Hammerspoon + Client Daemon Architecture
For comprehensive documentation, see:
- `ARCHITECTURE_INDEX.md` - Overview and references
- `HAMMERSPOON_ARCHITECTURE.md` - Detailed architecture
- `ARCHITECTURE_EXPLORATION_SUMMARY.txt` - Condensed findings
