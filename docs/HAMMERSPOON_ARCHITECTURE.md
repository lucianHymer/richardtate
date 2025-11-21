# Hammerspoon + Client Daemon Architecture

**Status**: Complete exploration and documentation
**Last Updated**: 2025-11-19

## Overview

The dictation system uses Hammerspoon as a macOS-specific UI layer that communicates with a Go client daemon running as an HTTP server. The daemon manages WebRTC connections to the transcription server, audio capture, and text insertion.

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                          macOS Application                          │
├─────────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────────────┐  │
│  │  Hammerspoon (Lua)                                           │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │  init.lua                                              │  │  │
│  │  │  - Hotkey bindings (Ctrl+N, Ctrl+Alt+C)               │  │  │
│  │  │  - Visual indicator (canvas)                          │  │  │
│  │  │  - HTTP control (POST /start, /stop)                 │  │  │
│  │  │  - WebSocket streaming (ws://localhost:8081/trans)   │  │  │
│  │  │  - Text insertion (hs.eventtap.keyStrokes)           │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  │  ┌────────────────────────────────────────────────────────┐  │  │
│  │  │  calibration.lua                                       │  │  │
│  │  │  - 3-step wizard UI (canvas)                          │  │  │
│  │  │  - HTTP endpoints for calibration                     │  │  │
│  │  │  - Background/speech recording                        │  │  │
│  │  │  - Threshold calculation & save                       │  │  │
│  │  └────────────────────────────────────────────────────────┘  │  │
│  └────────────────────┬──────────────────┬──────────────────────┘  │
│                       │                  │                          │
│  ╔════════════════════╩══════════════════╩══════════════════════╗  │
│  ║  HTTP Control (Port 8081)    WebSocket (Port 8081)          ║  │
│  ║  POST /start                 ws://localhost:8081/trans      ║  │
│  ║  POST /stop                  (streaming chunks)             ║  │
│  ║  POST /api/calibrate/*                                      ║  │
│  ╚════════════════════╦══════════════════╦══════════════════════╝  │
└───────────────────────┼──────────────────┼──────────────────────────┘
                        │                  │
         ┌──────────────▼──────────────────▼──────────────┐
         │  Client Daemon (Go)                           │
         │  client/cmd/client/main.go                    │
         │  ┌────────────────────────────────────────┐   │
         │  │  API Server (api/server.go)            │   │
         │  │  - HTTP endpoints for control          │   │
         │  │  - WebSocket client management         │   │
         │  │  - Audio capture for calibration       │   │
         │  │  - Transcription broadcasting          │   │
         │  └────────────────────────────────────────┘   │
         │                    ▲                           │
         │  ┌──────────────┬──┴──┬──────────────┐         │
         │  │              │     │              │         │
         │  ▼              ▼     ▼              ▼         │
         │ [Audio]       [WebRTC]         [Message       │
         │ [Capture]     [Client]         [Handler]      │
         │  │              │                  │         │
         │  │ Send chunks   │ Control          │ Receive│
         │  │ via WebRTC    │ start/stop       │ transcr│
         │  │              │ Audio streaming  │ results│
         │  │              │                  │         │
         └──┼──────────────┼──────────────────┼─────────┘
            │              │                  │
    ╔═══════╩══════════════╩══════════════════╩════════════╗
    ║              WebRTC (Audio + Control)               ║
    ║   ws://server:port/api/v1/stream/signal             ║
    ║   DataChannel: audio + transcription messages       ║
    ╚═══════════════════╦═════════════════════════════════╝
                        │
         ┌──────────────▼──────────────┐
         │  Transcription Server       │
         │  (Go - server/cmd/server)   │
         │  - WebRTC signaling         │
         │  - Audio processing         │
         │  - Whisper/Parakeet ASR     │
         │  - Transcription results    │
         └─────────────────────────────┘
```

## Detailed Data Flow

### Recording Session (User Presses Ctrl+N)

```
1. User presses Ctrl+N
   └─> Hammerspoon hotkey handler (toggleRecording)

2. Hammerspoon sends HTTP POST /start
   └─> Client daemon receives at api/server.go:handleStart()

3. Client daemon onStart handler:
   a) Initialize session state
      - sessionChunks = []
      - sessionStart = now
      - sessionRecording = true
   
   b) Call webrtcClient.SendControlStart()
      - Serialize VAD settings from config
      - Send via DataChannel: MessageTypeControlStart
   
   c) Call capturer.Start()
      - Audio device starts capturing

4. Audio capture loop (main.go, line 125-137):
   for chunk := range capturer.Chunks() {
      webrtcClient.SendAudioChunk(chunk.Data, ...)
   }

5. WebRTC sends audio chunks to server:
   - Serialize to protocol.AudioChunkData
   - Send as MessageTypeAudioChunk via DataChannel
   - Sequence ID incremented per chunk

6. Server processes audio:
   - Receives chunks
   - Transcribes via Whisper/Parakeet
   - Sends back MessageTypeTranscriptFinal

7. Client receives transcription:
   handleMessage() → handleDataChannelMessage()
   
8. Client broadcasts to Hammerspoon:
   apiServer.BroadcastTranscription(text, isFinal)
   
9. Hammerspoon receives via WebSocket:
   hs.websocket.new(ws://localhost:8081/transcriptions)
   conn.ReadMessage() → {"chunk": "text", "final": true}

10. Hammerspoon inserts text:
    hs.eventtap.keyStrokes(text .. " ")
    (Inserts at cursor with trailing space)

11. User presses Ctrl+N again
    └─> Hammerspoon calls HTTP POST /stop

12. Client daemon onStop handler:
    a) Call capturer.Stop()
       - Audio device stops
       - Closes chunks channel
    
    b) Log complete session:
       - Accumulate sessionChunks
       - Calculate duration
       - Call globalDebugLog.LogComplete()
    
    c) Send MessageTypeControlStop
       - Notify server session ended
    
    d) HTTP response returns 200 OK

13. Hammerspoon disconnects WebSocket:
    hs.timer.doAfter(1.0, disconnectWebSocket)
    (1 second delay allows final chunks to arrive)
```

### Calibration Session (User Presses Ctrl+Alt+C)

```
1. User presses Ctrl+Alt+C
   └─> Hammerspoon calls calibration.show()

2. Hammerspoon displays Step 1 (Background Recording)
   - Canvas window opens centered on screen
   - User clicks "Start Recording" button

3. User stays silent for 5 seconds
   Hammerspoon sends HTTP POST /api/calibrate/record
   └─> api/server.go:handleCalibrateRecord()
   
   Client endpoint:
   a) Initialize audio capture
      capturer := audio.New(...)
   b) Record for duration_seconds (5s)
   c) Collect all audio bytes
   d) Send to server: POST /api/v1/analyze-audio
      - Server analyzes energy
      - Returns: min, max, avg, p5, p95
   e) Return stats to Hammerspoon

4. Hammerspoon receives stats:
   response = {min: 12.3, max: 89.4, avg: 45.2, p5: 34.5, p95: 78.1}
   state.backgroundStats = response
   state.currentStep = 2
   Display Step 2 UI

5. User speaks for 5 seconds
   Same process as Step 1, returns speech stats

6. Hammerspoon calculates threshold:
   HTTP POST /api/calibrate/calculate
   └─> api/server.go:handleCalibrateCalculate()
   
   Calculation:
   recommendedThreshold = backgroundStats.P95 * 1.5
   (balances false positives and false negatives)
   
   Returns: {threshold: 117.15, background_frames_above: 5, speech_frames_above: 95}

7. Hammerspoon displays Step 3 (Results)
   - Shows stats and recommended threshold
   - Visual comparison bars
   - Save and Cancel buttons

8. User clicks "Save & Close"
   HTTP POST /api/calibrate/save
   └─> api/server.go:handleCalibrateSave()
   
   Server endpoint:
   a) Call config.UpdateVADThreshold(configPath, threshold)
      - Updates client config file YAML
      - Saves to disk
   b) Call cfg.Reload()
      - Reloads config in-memory
      - Updates all references to same Config struct
   c) Return success response

9. Hammerspoon receives success:
   hs.notify.new() - shows notification
   calibration.close() - closes wizard
   
   Client is now using new threshold on next recording
   (No restart required - config reloaded)
```

## Component Details

### Hammerspoon (init.lua) - 215 lines

**Hotkey Bindings**:
- `Ctrl+N`: toggleRecording() - starts/stops recording
- `Ctrl+Alt+C`: calibration.show() - opens calibration wizard
- `Cmd+Alt+Ctrl+R`: Reloads Hammerspoon with cleanup

**HTTP Control Functions**:
```lua
httpRequest(method, path, callback)
startRecording()  -- POST /start, show indicator, connect WebSocket
stopRecording()   -- POST /stop, hide indicator, disconnect WebSocket (1s delay)
```

**WebSocket Streaming**:
```lua
connectWebSocket()  -- ws://localhost:8081/transcriptions
-- Handles three event types:
-- "received": chunk message with JSON {"chunk": "text"}
-- "open": Connected
-- "closed": Disconnected
-- "fail": Connection failed
```

**Text Insertion**:
```lua
-- Received WebSocket message {"chunk": "Hello world"}
hs.eventtap.keyStrokes(text .. " ")
-- Types at cursor position with trailing space
```

**Indicator UI**:
- Canvas-based floating window
- Position: top-right corner (20px margin)
- Size: 200x40 pixels
- Content: Red dot + "Recording..." text
- Colors: Dark background (RGB 0.1, 0.1, 0.1), white text

### Hammerspoon Calibration (calibration.lua) - 490 lines

**3-Step Wizard**:

Step 1: Background Recording
- Title: "🎤 VAD Calibration - Step 1/3"
- Subtitle: "Background Noise" (blue color)
- Instructions: "Stay completely silent..."
- Button: "Start Recording"
- On click: recordAudio() → POST /api/calibrate/record

Step 2: Speech Recording
- Title: "🎤 VAD Calibration - Step 2/3"
- Subtitle: "Speech Recording" (orange color)
- Show previous background stats
- Button: "Start Recording"
- On click: recordAudio() → POST /api/calibrate/record

Step 3: Results
- Title: "🎤 VAD Calibration - Step 3/3"
- Subtitle: "Results" (green color)
- Display both stats side-by-side
- Visual bars showing energy comparison
- Recommended threshold (large font)
- Buttons: "Save & Close" and "Cancel"
- On Save click: POST /api/calibrate/save

**Recording Progress**:
```lua
recordAudio(callback)
  state.isRecording = true
  timer = hs.timer.doEvery(0.5, updateUI)
  -- Updates progress every 0.5 seconds
  httpRequest("POST", "/api/calibrate/record", {duration_seconds = 5}, callback)
```

**HTTP Helper**:
```lua
httpRequest(method, path, body, callback)
  url = config.daemonURL .. path
  hs.http.doAsyncRequest(url, method, bodyData, headers, callback)
  -- Parses JSON response or error
```

### Client Daemon (client/cmd/client/main.go) - 310 lines

**Main Function**:
1. Parse flags: --config, --calibrate, --yes
2. Load configuration
3. Initialize logger and debug log
4. Create WebRTC client
5. Connect to server
6. Wait for DataChannel
7. Create audio capturer
8. Start audio sending goroutine
9. Create and start API server
10. Wait for interrupt signal
11. Shutdown cleanup

**Session State** (global variables):
```go
var (
    sessionMu        sync.Mutex
    sessionChunks    []string         // Accumulated transcription text
    sessionStart     time.Time        // Session start time
    sessionRecording bool             // Recording state
)
```

**Start Recording** (API handler):
```go
onStart := func() error {
    sessionMu.Lock()
    sessionChunks = []string{}
    sessionStart = time.Now()
    sessionRecording = true
    sessionMu.Unlock()
    
    webrtcClient.SendControlStart()  // Notify server
    capturer.Start()                 // Begin audio capture
    return nil
}
```

**Stop Recording** (API handler):
```go
onStop := func() error {
    capturer.Stop()                  // Stop audio capture
    
    sessionMu.Lock()
    sessionRecording = false
    fullText := strings.Join(sessionChunks, " ")
    duration := time.Since(sessionStart).Seconds()
    sessionMu.Unlock()
    
    globalDebugLog.LogComplete(fullText, duration)  // Log session
    webrtcClient.SendControlStop()                  // Notify server
    return nil
}
```

**Message Handler**:
```go
handleDataChannelMessage(msg *protocol.Message)
  switch msg.Type {
  case MessageTypeTranscriptFinal:
      var transcript protocol.TranscriptData
      json.Unmarshal(msg.Data, &transcript)
      
      fmt.Printf("✅ %s\n", transcript.Text)
      
      // Broadcast to Hammerspoon
      globalAPIServer.BroadcastTranscription(transcript.Text, true)
      
      // Log to debug log
      globalDebugLog.LogChunk(transcript.Text)
      
      // Accumulate for session
      sessionMu.Lock()
      if sessionRecording {
          sessionChunks = append(sessionChunks, transcript.Text)
      }
      sessionMu.Unlock()
  }
```

### API Server (client/internal/api/server.go) - 549 lines

**HTTP Endpoints**:

| Endpoint | Method | Handler | Purpose |
|----------|--------|---------|---------|
| /health | GET | handleHealth | Health check with timestamp |
| /start | POST | handleStart | Start recording |
| /stop | POST | handleStop | Stop recording |
| /status | GET | handleStatus | Get recording status |
| /transcriptions | GET | handleTranscriptions | WebSocket upgrade |
| /api/calibrate/record | POST | handleCalibrateRecord | Record calibration audio |
| /api/calibrate/calculate | POST | handleCalibrateCalculate | Calculate threshold |
| /api/calibrate/save | POST | handleCalibrateSave | Save config |

**WebSocket Management**:
```go
type Server struct {
    wsClients   map[*websocket.Conn]bool  // Connected clients
    wsClientsMu sync.RWMutex
    wsUpgrader  websocket.Upgrader
}

func (s *Server) handleTranscriptions(w http.ResponseWriter, r *http.Request) {
    conn, _ := s.wsUpgrader.Upgrade(w, r, nil)
    s.wsClientsMu.Lock()
    s.wsClients[conn] = true
    s.wsClientsMu.Unlock()
    
    // Keep connection alive
    for {
        if _, _, err := conn.ReadMessage(); err != nil {
            break
        }
    }
    
    // Cleanup on disconnect
    s.wsClientsMu.Lock()
    delete(s.wsClients, conn)
    s.wsClientsMu.Unlock()
}

func (s *Server) BroadcastTranscription(text string, isFinal bool) {
    message := map[string]interface{}{"chunk": text, "final": isFinal}
    data, _ := json.Marshal(message)
    
    s.wsClientsMu.RLock()
    defer s.wsClientsMu.RUnlock()
    
    for conn := range s.wsClients {
        conn.WriteMessage(websocket.TextMessage, data)
    }
}
```

**Calibration Recording**:
```go
func (s *Server) handleCalibrateRecord(w http.ResponseWriter, r *http.Request) {
    var req CalibrateRecordRequest
    json.NewDecoder(r.Body).Decode(&req)  // duration_seconds: 5
    
    capturer, _ := audio.New(20, s.cfg.Audio.DeviceName, s.baseLog)
    defer capturer.Close()
    
    capturer.Start()
    
    // Collect audio for 5 seconds
    var allAudio []byte
    endTime := time.Now().Add(5 * time.Second)
    for time.Now().Before(endTime) {
        select {
        case chunk := <-capturer.Chunks():
            allAudio = append(allAudio, chunk.Data...)
        case <-time.After(100 * time.Millisecond):
        }
    }
    
    // Send to server for analysis
    stats, _ := s.analyzeAudio(allAudio)
    
    // Return statistics
    json.NewEncoder(w).Encode(stats)
}
```

**Threshold Calculation**:
```go
func (s *Server) handleCalibrateCalculate(w http.ResponseWriter, r *http.Request) {
    var req CalibrateCalculateRequest
    json.NewDecoder(r.Body).Decode(&req)
    
    // Calculate: background P95 * 1.5
    recommendedThreshold := req.Background.P95 * 1.5
    
    // Safety check
    minThreshold := req.Background.Avg * 2
    if recommendedThreshold < minThreshold {
        recommendedThreshold = minThreshold
    }
    
    response := CalibrateCalculateResponse{
        Threshold: recommendedThreshold,
    }
    
    json.NewEncoder(w).Encode(response)
}
```

**Config Save with Hot-Reload**:
```go
func (s *Server) handleCalibrateSave(w http.ResponseWriter, r *http.Request) {
    var req CalibrateSaveRequest
    json.NewDecoder(r.Body).Decode(&req)  // threshold: 117.15
    
    // Update config file
    config.UpdateVADThreshold(s.configPath, req.Threshold)
    
    // Reload config in-memory (updates all references)
    s.cfg.Reload()
    
    response := CalibrateSaveResponse{Success: true}
    json.NewEncoder(w).Encode(response)
}
```

### WebRTC Client (client/internal/webrtc/client.go)

**Core Functionality**:

1. **Connection Establishment**:
   - WebSocket signaling: ws://server/api/v1/stream/signal
   - DataChannel creation: Ordered, reliable (unlimited retransmits)
   - Handshake: Offer/Answer exchange via signaling

2. **Audio Sending**:
```go
func (c *Client) SendAudioChunk(data []byte, sampleRate, channels int) error {
    seqID := c.sequenceID++
    
    c.connectedMu.RLock()
    connected := c.connected
    c.connectedMu.RUnlock()
    
    c.reconnectingMu.RLock()
    reconnecting := c.reconnecting
    c.reconnectingMu.RUnlock()
    
    // If disconnected but reconnecting, buffer chunk
    if !connected && reconnecting {
        c.bufferChunk(data, sampleRate, channels, seqID)
        return nil
    }
    
    // If disconnected and not reconnecting, error
    if !connected {
        return fmt.Errorf("not connected")
    }
    
    // Send immediately
    audioData := protocol.AudioChunkData{
        SampleRate: sampleRate,
        Channels: channels,
        Data: data,
        SequenceID: seqID,
    }
    
    audioJSON, _ := json.Marshal(audioData)
    msg := &protocol.Message{
        Type: MessageTypeAudioChunk,
        Data: audioJSON,
    }
    
    return c.SendMessage(msg)
}
```

3. **Message Reception**:
```go
func (c *Client) handleMessage(data []byte) {
    var msg protocol.Message
    json.Unmarshal(data, &msg)
    
    // Dispatch to callback
    if c.onMessage != nil {
        c.onMessage(&msg)
    }
}
```

4. **Reconnection**:
   - Exponential backoff: 1s, 2s, 4s, 8s, 16s, 30s (max)
   - Auto-buffering during reconnection
   - Buffer flush after reconnection
   - Max 10 attempts

## Message Formats

### Hammerspoon → Client (HTTP)

**POST /start** (no body):
```
HTTP/1.1 200 OK
Content-Type: application/json

{"status": "started"}
```

**POST /stop** (no body):
```
HTTP/1.1 200 OK
Content-Type: application/json

{"status": "stopped"}
```

**POST /api/calibrate/record**:
```json
{
  "duration_seconds": 5
}
```

Response:
```json
{
  "min": 12.3,
  "max": 89.4,
  "avg": 45.2,
  "p5": 34.5,
  "p95": 78.1,
  "sample_count": 500
}
```

**POST /api/calibrate/calculate**:
```json
{
  "background": {"min": 12.3, "max": 89.4, "avg": 45.2, "p5": 34.5, "p95": 78.1},
  "speech": {"min": 234.5, "max": 1823.7, "avg": 654.3, "p5": 290.2, "p95": 1456.8}
}
```

Response:
```json
{
  "threshold": 117.15,
  "background_frames_above_percent": 5,
  "speech_frames_above_percent": 95,
  "explanation": "Calculated as background P95 (78.1) × 1.5 for 50% safety margin"
}
```

**POST /api/calibrate/save**:
```json
{
  "threshold": 117.15
}
```

Response:
```json
{
  "success": true,
  "config_path": "~/.config/richardtate/client.yaml"
}
```

### Hammerspoon ← Client (WebSocket)

**Message Format**:
```json
{
  "chunk": "Transcribed text here",
  "final": true
}
```

**Event Types**:
- `received`: Message with chunk and final flag
- `open`: WebSocket connected
- `closed`: WebSocket closed
- `fail`: Connection failed

### Client ↔ Server (WebRTC DataChannel)

**Control Start** (Client → Server):
```go
type ControlStartData struct {
    VADEnergyThreshold    float64
    SilenceThresholdMs    int
    MinChunkDurationMs    int
    MaxChunkDurationMs    int
    SpeechDensityThreshold float64
}

// Sent as: MessageTypeControlStart
```

**Audio Chunk** (Client → Server):
```go
type AudioChunkData struct {
    SampleRate int
    Channels   int
    Data       []byte  // PCM int16 samples
    SequenceID uint64
}

// Sent as: MessageTypeAudioChunk
```

**Transcription Result** (Server → Client):
```go
type TranscriptData struct {
    Text string
}

// Received as: MessageTypeTranscriptFinal or MessageTypeTranscriptPartial
```

## Key Design Decisions

### 1. Hammerspoon as UI Layer
- **Why**: macOS-native, works system-wide, requires no additional UI framework
- **Trade-off**: macOS-only (not cross-platform)
- **Alternative**: Could be replaced with other UIs (web, Electron, etc.) via HTTP/WebSocket

### 2. Direct Text Insertion (Not Preview)
- **Why**: Simplicity, magical UX, works in any app
- **Trade-off**: No preview/edit before insertion
- **Could change in V2**: Add preview mode while keeping direct insertion

### 3. HTTP Control + WebSocket Streaming
- **Why**: Separation of concerns, standard protocols
- **Control (HTTP)**: Synchronous, request-response for start/stop
- **Streaming (WebSocket)**: Asynchronous, server-push for transcription chunks

### 4. Config Hot-Reload After Calibration
- **Why**: Seamless workflow, no need to restart daemon
- **How**: cfg.Reload() updates in-place, all references see new values

### 5. Session Tracking in Client Daemon
- **Why**: Simplifies logging, tracks complete sessions
- **Location**: sessionChunks array in main.go
- **Logged to**: Debug log on session stop

### 6. Per-WebSocket-Client Broadcasting
- **Why**: Multiple Hammerspoon instances could theoretically connect
- **Reality**: Currently one instance, but architecture supports multiple

## Threading Model

**Goroutines**:
1. **Main thread**: Initialization, signal handling, cleanup
2. **Audio capture**: Infinite loop reading audio chunks, sending to server
3. **WebRTC signaling**: Handles offer/answer/ICE candidates
4. **API server**: Handles HTTP requests (one goroutine per request)
5. **WebSocket message handler**: Reads from WebSocket (one per connection)
6. **Reconnection**: Runs when connection lost, attempts exponential backoff

**Synchronization**:
- `sync.Mutex`: Protects sessionMu, wsClientsMu, etc.
- `sync.RWMutex`: Protects connectedMu, reconnectingMu
- Channels: Used for audio chunk pipeline

## Performance Characteristics

### Latency
- **Hammerspoon hotkey → daemon**: < 100ms (HTTP overhead)
- **Daemon → server (audio)**: ~50ms per 200ms chunk (network RTT)
- **Server → client (transcription)**: Depends on model (1-3 seconds)
- **Client → Hammerspoon (WebSocket)**: < 10ms (local WebSocket)
- **Total user-perceived latency**: 1-3 seconds (dominated by transcription)

### Memory Usage
- **Hammerspoon**: ~50MB (Lua runtime)
- **Client daemon**: ~100MB (Go runtime + audio buffers)
- **WebRTC connection**: ~50MB (peer connection state)
- **Session buffer**: ~1MB (1000 chunks × ~1KB each)

### CPU Usage
- **Idle**: < 1% (just event loop)
- **Recording**: 5-10% (audio capture + WebRTC sending)
- **Transcription**: Varies (handled by server)

## Future Enhancements

### V2 Possibilities
1. **Preview mode**: Show transcription before inserting
2. **Processing modes**: Casual, professional, technical styles
3. **Multi-language**: Language selection UI
4. **Cross-platform**: Replace Hammerspoon with web UI
5. **Offline mode**: Local transcription if server unavailable

### Architecture Changes
1. **Streaming UI**: WebSocket for real-time energy feedback during calibration
2. **Insertion persistence**: Save text to file if insertion fails
3. **Session management**: Server-side session tracking
4. **Load balancing**: Multiple server support

## Summary

The Hammerspoon + Client Daemon architecture provides a clean separation between UI and logic:

- **Hammerspoon** handles all user interaction, visual feedback, and text insertion
- **Client daemon** manages audio capture, WebRTC communication, and transcription streaming
- **HTTP + WebSocket** standardizes communication, allowing future UI implementations
- **Hot-reload config** provides seamless user workflow for calibration
- **WebRTC** enables robust audio streaming with automatic reconnection

This design allows Hammerspoon to be replaced with alternative UIs (web, Electron, iOS app, etc.) without changing the core client daemon or server.
