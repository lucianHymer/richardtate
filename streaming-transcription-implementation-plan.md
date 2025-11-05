# Richardtate - Real-Time Voice Streaming Transcription System

Dictate got classy.

## Implementation Plan

## Executive Summary

Build a real-time voice transcription system that streams audio to a server, processes it through RNNoise for noise removal, uses VAD for intelligent chunking, transcribes with Whisper, and offers optional post-processing through Claude Haiku. The system replaces the current file-based approach with streaming for near-instant feedback.

## Current System Overview

### What We Have Now
- **Hammerspoon script** (Lua) that:
  - Records audio to file using `sox`
  - Applies RNNoise post-recording via `ffmpeg`
  - Transcribes complete file with `whisper.cpp`
  - Optional Claude post-processing in "pro mode"
  - Hotkey: Ctrl+N to start/stop, 'n' during recording toggles pro mode

### Limitations of Current System
- Must wait for entire recording to finish before processing
- No real-time feedback during speaking
- All processing happens sequentially after recording stops
- File-based approach adds unnecessary I/O overhead

## Target System Architecture

### High-Level Design

#### V1: Core Streaming System
```
┌──────────────┐   WebRTC DataChannel   ┌──────────────┐   Transcription  ┌──────────────┐
│   Client     │ ─────[Audio Chunks]───> │   Server     │ ──[Text Chunks]─> │   Client     │
│ (Go Daemon)  │ <────[Text Chunks]───── │ (Go Service) │                   │  (UI Window) │
└──────────────┘    (Reliable, Ordered)  └──────────────┘                   └──────────────┘
                                                                                    │
                                                                                    ↓
                                                                           [Debug Log File]
                                                                            (Append only)

Features:
- Real-time streaming transcription
- Network-resilient connection  
- Debug logging for recovery
- Raw text insertion
```

#### V2: Post-Processing Addition (Future)
```
After recording stops:
┌──────────────┐     Complete Text      ┌──────────────┐   Claude API     ┌──────────────┐
│  UI Window   │ ────────────────────>  │   Server     │ ──────────────>  │   Claude     │
│              │ <────[Formatted]─────   │ (Go Service) │ <──[Response]──  │   (Haiku)    │
└──────────────┘     REST API Call      └──────────────┘                   └──────────────┘

Additional features:
- 5 processing modes
- Claude Haiku integration
- Instant mode switching
```

### Component Breakdown

#### 1. Client Daemon (Go)
- Local service running on port 8888
- Captures audio from system microphone
- Streams to server via WebRTC DataChannels
- Receives transcriptions and forwards to UI
- HTTP API for Hammerspoon control

#### 2. Server (Go) 
- WebRTC endpoint for audio streaming
- Real-time audio processing pipeline
- REST endpoint for text post-processing
- Manages multiple concurrent streams

#### 3. UI Window (HTML/JS in Hammerspoon WebView)
- Displays streaming transcriptions
- Shows post-processing options
- Handles user interactions
- Inserts final text at cursor position

## Detailed Technical Specifications

### Understanding WebSockets vs WebRTC DataChannels

#### Why Both?
- **WebSocket**: Used ONLY for initial WebRTC signaling (connection setup)
- **DataChannel**: Used for ALL actual data transfer (audio and transcriptions)

#### Connection Flow
1. Client connects to server via WebSocket
2. WebSocket exchanges ICE candidates, SDP offers/answers (connection details)
3. WebRTC DataChannel establishes direct connection
4. WebSocket can close - DataChannel continues working
5. DataChannel handles all audio/transcription streaming

#### Why DataChannels Are Superior
- **Connection Resilience**: Survives network changes that would kill WebSockets
- **Mobile-Friendly**: Handles WiFi ↔ Cellular transitions seamlessly  
- **NAT Traversal**: Works through complex network configurations
- **Automatic Recovery**: Built-in reconnection without manual state management

### Complete Streaming and Processing Workflow

```
USER STARTS RECORDING (Ctrl+N)
         ↓
1. STREAMING PHASE (Real-time)
   Audio captured in 100-200ms chunks
         ↓
   Each chunk sent via DataChannel (reliable mode)
         ↓
   Server processes chunk through pipeline
         ↓
   Transcription sent back via DataChannel
         ↓
   Client displays chunk immediately
         ↓
   Client accumulates all chunks in memory
         ↓
   [Repeat until user stops recording]

USER STOPS RECORDING
         ↓
2. POST-PROCESSING PHASE (On-demand)
   Client has complete text (all chunks concatenated)
         ↓
   User presses mode key (E/I/O/U/Y)
         ↓
   Complete text sent to /api/v1/process endpoint
         ↓
   Claude processes ENTIRE text with mode-specific prompt
         ↓
   Formatted text returned to client
         ↓
   Displayed in bottom panel
```

**Critical Points**:
- Chunks are for real-time display only
- No chunks are ever lost (reliable DataChannel)
- Post-processing always uses complete text
- Claude never sees individual chunks

### Audio Pipeline

```
Microphone Input (16kHz mono PCM)
         ↓
Client-side buffering (100-200ms chunks)
         ↓
WebRTC DataChannel (reliable mode, ordered)
         ↓
Server receives audio chunks (guaranteed delivery)
         ↓
RNNoise Processing (frame-by-frame, 10ms)
         ↓
Voice Activity Detection (VAD)
         ↓
Intelligent Chunking (on 500-800ms silence)
         ↓
Whisper Transcription (per chunk)
         ↓
Stream transcription back to client (reliable)
         ↓
Client accumulates ALL chunks for display
```

**Important**: WebRTC DataChannels in reliable mode ensure every chunk arrives in order. This gives us:
- **No missing words** - 100% of audio is transcribed
- **Connection resilience** - Survives WiFi switches, mobile network changes
- **Automatic recovery** - No manual reconnection logic needed
- **Real-time display** - Show chunks as they arrive, accumulating the full text

### Server API Specification

#### WebRTC Streaming Endpoint
- **Signaling**: `/api/v1/stream/signal` (WebSocket for connection setup)
- **DataChannel**: **Reliable mode, ordered** (ensures no chunks are lost)
- **Audio format**: 16kHz, mono, 16-bit PCM
- **Chunk size**: 100-200ms of audio data
- **Connection resilience**: Survives network switches, WiFi changes, mobile roaming

#### REST API Endpoints (V1)

##### GET `/api/v1/health`
- Returns server status, active streams count
- Format: `{"status": "healthy", "active_streams": 3, "uptime_seconds": 14523}`

### Client Daemon Specification

#### Debug Logging
- **Rolling log file**: `~/.streaming-transcription/debug.log`
- **Max size**: 8MB (easily holds 500k+ words, several hours of conversation)
- **Rotation**: When hitting 8MB, rotate to `debug.log.1` and start fresh
- **Write behavior**: Append each chunk immediately as received (sync write for safety)
- **Format**: Timestamped JSON for easy parsing
```json
{"timestamp": "2024-01-15T10:23:45.123Z", "type": "chunk", "text": "this is what was transcribed", "chunk_id": 42}
{"timestamp": "2024-01-15T10:23:47.456Z", "type": "complete", "full_text": "entire concatenated transcription", "duration_seconds": 45}
{"timestamp": "2024-01-15T10:23:48.789Z", "type": "inserted", "location": "Obsidian", "length": 523}
```
- **Purpose**: 
  - Recovery if UI crashes
  - Personal searchable archive
  - Debug transcription issues
  - Never lose a thought
- **Implementation note**: Use buffered writer with flush on each chunk
- **Privacy**: Add `.gitignore` for this file, never upload logs

#### HTTP Control API (Port 8888)

##### POST `/start`
- Begins audio capture and streaming
- Returns: `{"status": "streaming", "session_id": "uuid"}`

##### POST `/stop`  
- Stops audio capture
- Returns: `{"status": "stopped"}`

##### POST `/mode`
```json
Request: {"mode": "raw|pro"}
Response: {"status": "mode_changed", "mode": "pro"}
```

##### WebSocket `/transcriptions`
- Local WebSocket for UI communication (not for server streaming)
- Daemon forwards transcriptions received via DataChannel to UI
- Format: `{"chunk": "text", "timestamp": 1234567890, "final": false}`
- When recording stops: `{"full_text": "complete transcription", "final": true}`

### UI Window Specification

#### Layout
```
┌─────────────────────────────────────────────┐
│  Raw Transcription (streaming)               │
│  ┌─────────────────────────────────────────┐ │
│  │                                          │ │
│  │  [Streaming text appears here...]        │ │
│  │                                          │ │
│  └─────────────────────────────────────────┘ │
│                                              │
│  ╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌╌ │
│                                              │
│  Actions (Preview - Coming Soon):            │
│  [N] Raw   [E] Casual   [I] Professional    │
│  [O] Obsidian   [U] Code   [Y] Email        │
│  (Currently disabled - streaming only)       │
│                                              │
│  Processed Output:                          │
│  ┌─────────────────────────────────────────┐ │
│  │                                          │ │
│  │  [Preview area - not yet active]         │ │
│  │                                          │ │
│  └─────────────────────────────────────────┘ │
│                                              │
│  [↵] Insert Raw   [⌘C] Copy   [ESC] Cancel  │
│                                              │
│  ● Recording  [00:42]                       │
└─────────────────────────────────────────────┘
```

**V1 Behavior**: 
- Only raw streaming works
- Processing buttons visible but greyed out
- Enter inserts raw transcription
- Bottom panel shows "Preview - processing modes coming soon"

#### Window Behavior
- Opens on Ctrl+N hotkey
- Stays on top but doesn't steal focus
- Position: Near current cursor or top-right of screen
- Size: 600x500px (resizable)
- Auto-closes on Insert or Cancel

## Implementation Phases

### V1 Timeline: 3-4 Weeks
Focus exclusively on perfect streaming with debug logging. Ship when rock-solid.

### Phase 1: Core Infrastructure (Week 1)
**Goal**: Basic client-server communication working

#### 1.1 Server Setup
- [ ] Create Go project structure
- [ ] Implement WebRTC signaling server
- [ ] Set up DataChannel handling
- [ ] Create basic health endpoint
- [ ] Add structured logging

#### 1.2 Client Daemon
- [ ] Create Go daemon project
- [ ] Implement audio capture (using portaudio or similar)
- [ ] WebRTC client implementation
- [ ] HTTP control API
- [ ] **Debug logging system (8MB rolling log)**
- [ ] Basic error handling and reconnection

#### 1.3 Testing
- [ ] Unit tests for audio capture
- [ ] Integration tests for WebRTC connection establishment
- [ ] Test DataChannel reliable delivery (no dropped chunks)
- [ ] Test connection resilience (WiFi switches, network changes)
- [ ] Verify chunk ordering is preserved
- [ ] Test reconnection during mobile network transitions
- [ ] **Verify debug log rotation at 8MB**

### Phase 2: Audio Processing Pipeline (Week 2)
**Goal**: RNNoise + VAD + Chunking working

#### Implementation Notes: Reliable DataChannel Setup

```go
// Server-side DataChannel configuration
peerConnection.OnDataChannel(func(d *webrtc.DataChannel) {
    d.OnOpen(func() {
        // Configure for reliable, ordered delivery
        d.SetOrdered(true)           // Preserve chunk order
        d.SetMaxRetransmits(nil)     // Unlimited retries (fully reliable)
        d.SetMaxPacketLifeTime(nil)  // No timeout
        
        log.Printf("DataChannel opened with reliable delivery")
    })
    
    d.OnMessage(func(msg webrtc.DataChannelMessage) {
        // Process audio chunk - guaranteed to arrive in order
        audioChunk := msg.Data
        processAudioChunk(audioChunk)
    })
})

// Client-side configuration (similar)
dataChannel, _ := peerConnection.CreateDataChannel("audio", &webrtc.DataChannelInit{
    Ordered:        &[]bool{true}[0],  // Reliable, ordered
    MaxRetransmits: nil,                // Unlimited retries
})
```

#### 2.1 RNNoise Integration
- [ ] CGO bindings for RNNoise
- [ ] Streaming RNNoise processor
- [ ] State management across chunks
- [ ] Performance benchmarking

#### 2.2 VAD Implementation
- [ ] Integrate WebRTC VAD or Silero VAD
- [ ] Configurable silence thresholds
- [ ] Chunk boundary detection
- [ ] Handle edge cases (long silence, continuous speech)

#### 2.3 Buffer Management
- [ ] Implement ring buffer for audio
- [ ] Chunk accumulation logic
- [ ] Memory-efficient streaming

#### 2.4 Testing
- [ ] Test with various noise conditions
- [ ] Verify chunk boundaries are sensible
- [ ] Load test with multiple streams

### Phase 3: Transcription Integration (Week 3)
**Goal**: Whisper transcription with streaming results

#### 3.1 Whisper Integration
- [ ] Subprocess management for whisper.cpp
- [ ] Queue management for chunks
- [ ] Context preservation between chunks
- [ ] Error handling and retries

#### 3.2 Streaming Protocol
- [ ] Define WebSocket message protocol
- [ ] Implement chunked transcription streaming
- [ ] Add timestamps and metadata
- [ ] **Append all chunks to debug log**

#### 3.3 Testing
- [ ] Test transcription accuracy
- [ ] Verify streaming latency < 500ms
- [ ] Test with various accents/speaking speeds
- [ ] **Verify debug log contains all chunks**

### Phase 4: UI Implementation - V1 (Week 3-4)
**Goal**: Basic streaming UI with preview of future features

#### 4.1 Hammerspoon Integration
- [ ] WebView window creation
- [ ] Hotkey management (Ctrl+N)
- [ ] Communication with Go daemon

#### 4.2 UI Development - Streaming Only
- [ ] HTML/CSS layout with disabled processing buttons
- [ ] WebSocket client for transcriptions
- [ ] Display streaming text in real-time
- [ ] Insert raw text functionality (Enter key)
- [ ] Copy functionality (Cmd+C)
- [ ] Visual feedback (recording indicator)
- [ ] Show "Preview" labels on disabled features

#### 4.3 Testing
- [ ] Test Ctrl+N hotkey
- [ ] Verify text insertion works in various apps
- [ ] Test window positioning logic
- [ ] Verify streaming display updates smoothly

### Phase 5: Polish and Production - V1 (Week 5)
**Goal**: Production-ready streaming system

#### 5.1 Performance Optimization
- [ ] Profile and optimize hot paths
- [ ] Minimize latency throughout pipeline
- [ ] Memory usage optimization

#### 5.2 Reliability
- [ ] Automatic reconnection logic
- [ ] Graceful degradation
- [ ] Comprehensive error handling
- [ ] Audit logging

#### 5.3 Deployment
- [ ] SystemD service files
- [ ] LaunchAgent for macOS
- [ ] Installation script
- [ ] Documentation for V1 features

---

## V2 Preview: Post-Processing Features (Future Release)

### Overview
The post-processing features are designed but not implemented in V1. The UI shows these as "preview" to indicate future functionality. When V2 ships, users will be able to process their transcriptions through Claude Haiku for various formatting modes.

### Server Endpoints (V2)

#### POST `/api/v1/process`
```json
Request:
{
  "text": "complete transcription text (all chunks concatenated)",
  "mode": "casual|professional|obsidian|code|email"
}

Response:
{
  "processed": "formatted text",
  "mode": "casual",
  "processing_time_ms": 150
}
```

**Note**: Post-processing happens ONLY after streaming is complete. The full concatenated text from all chunks is sent to Claude for processing - never individual chunks.

### Claude API Integration (V2)
- Haiku model for fast processing
- Prompt templates for each mode
- Streaming responses for large texts
- Automatic retry on rate limits

### Processing Mode Specifications (V2)

### Processing Mode Specifications (V2)

#### Casual Mode
- Remove filler words (um, uh, like, you know)
- Fix basic grammar
- Maintain conversational tone

#### Professional Mode  
- Formal grammar and punctuation
- Remove all filler words
- Structure into paragraphs
- Business-appropriate language

#### Obsidian Mode
- Markdown formatting
- Detect and format lists
- Recognize tasks: `- [ ] task format`
- Add headers for topic changes
- WikiLink detection: `[[concept]]`

#### Code Mode
- Format as code comments
- Detect variable/function names
- Preserve technical terms exactly
- Add appropriate comment syntax

#### Email Mode
- Greeting and signature detection
- Professional tone
- Paragraph structure
- Action items highlighted

### UI Behavior in V2
When V2 ships, the processing buttons will be enabled:
- User presses mode key (E/I/O/U/Y) after recording
- Complete text sent to server for processing
- Processed version appears in bottom panel
- User can switch between modes instantly
- Raw text always preserved in top panel

---

## Testing Strategy

### V1 Testing Requirements

#### Unit Tests Required
- Audio capture and buffering
- RNNoise processing
- VAD logic and state machines
- Chunk boundary detection
- WebRTC connection handling
- Debug log rotation at 8MB
- JSON log format validation

#### Integration Tests Required
- End-to-end audio streaming with reliable delivery verification
- Verify all chunks arrive and are displayed in order
- Test no data loss during network transitions
- Debug log contains every chunk with proper timestamps
- Transcription accuracy on concatenated chunks
- UI displays streaming text smoothly
- Raw text insertion into various applications
- Network failure recovery with automatic reconnection
- Mobile device roaming scenarios (WiFi ↔ Cellular)

#### Performance Requirements (V1)
- Transcription latency: < 500ms per chunk
- Support 10+ concurrent streams
- Memory usage: < 200MB per stream
- CPU usage: < 20% for single stream
- Debug log write latency: < 10ms per chunk

### V2 Testing Requirements (Future)

#### Additional Tests for V2
- Post-processing on complete text for each mode
- Verify Claude only processes full text, never partial chunks
- Claude API error handling and retries
- Processing mode accuracy for all 5 modes
- Response caching functionality

## Configuration Management

### Server Config (YAML)
```yaml
server:
  port: 8080
  workers: 4

audio:
  sample_rate: 16000
  chunk_duration_ms: 100
  
vad:
  silence_threshold_ms: 500
  energy_threshold: 0.01
  
whisper:
  model_path: "/models/ggml-large-v3-turbo.bin"
  language: "en"
  threads: 8
  
claude:
  api_key: "${CLAUDE_API_KEY}"
  model: "claude-3-haiku-20240307"
  max_retries: 3
```

### Client Config
```yaml
daemon:
  port: 8888
  server_url: "ws://localhost:8080"
  
audio:
  device: "default"
  buffer_size_ms: 200
  
debug_log:
  enabled: true
  path: "~/.streaming-transcription/debug.log"
  max_size_mb: 8
  sync_writes: true  # Flush immediately for safety
  
ui:
  window_width: 600
  window_height: 500
  position: "cursor" # or "top-right"
```

## Security Considerations

- Use TLS for all network communication in production
- Validate and sanitize all text before insertion
- Rate limit API requests per session
- Audio data should be ephemeral (not persisted)
- Add authentication for multi-user deployments

## Monitoring and Observability

### Metrics to Track
- Stream count and duration
- Transcription accuracy (via user feedback)
- Processing latencies (p50, p95, p99)
- Error rates by component
- Post-processing mode usage

### Logging
- Structured JSON logs
- Correlation IDs for request tracking
- Audio level monitoring
- Performance profiling hooks

## Migration Path

1. Run new system in parallel with existing Hammerspoon script
2. Add feature flag to switch between old and new
3. Gradual rollout with fallback option
4. Deprecate old system after stability confirmed

## Future Enhancements

### Near-term (Nice to have for V1)
- **Debug log viewer tool** - Simple CLI to tail, search, and export from debug.log
- WebSocket-only fallback mode for simpler deployments

### Long-term (V3+)
- Speaker diarization for multiple speakers
- Real-time translation to other languages
- Custom vocabulary and acronym support
- Voice commands for mode switching
- Mobile client support
- Partial transcription display while speaking
- Audio replay functionality
- Cloud deployment for remote access

## Success Criteria

### V1 - Streaming Only
- [ ] Streaming latency < 500ms end-to-end
- [ ] 100% chunk delivery (no missing transcription chunks)  
- [ ] Seamless handling of network transitions
- [ ] Debug log reliably captures all transcriptions
- [ ] 99% uptime for local service
- [ ] Clean installation process
- [ ] Raw text insertion works flawlessly
- [ ] UI shows streaming text in real-time
- [ ] Comprehensive test coverage (>80%)
- [ ] User documentation for V1 features

### V2 - Post-Processing (Future)
- [ ] Post-processing operates on complete text only
- [ ] All 5 processing modes working accurately
- [ ] Claude API integration with proper error handling
- [ ] Mode switching < 300ms response time
- [ ] Processing results cached appropriately

## Resources and Dependencies

### Required Libraries
- **Go**: gorilla/websocket, pion/webrtc, portaudio bindings
- **C/C++**: RNNoise, whisper.cpp
- **JavaScript**: Simple WebSocket client
- **External**: Claude API access

### Development Tools
- Go 1.21+
- CGO for C bindings
- Make for build automation
- Docker for testing environments

## Documentation Requirements

- API documentation (OpenAPI spec)
- Installation guide
- User manual with hotkey reference
- Troubleshooting guide
- Architecture diagrams
- Code comments and godoc

## Team Notes

### V1 Focus: Get Streaming Perfect
This system prioritizes **real-time feedback** first. V1 is all about nailing the streaming experience - users should see their words appearing as they speak with incredible reliability. Post-processing is intentionally deferred to V2.

**Key architectural decisions:**

1. **WebRTC DataChannels in reliable mode** - Guarantees no missing chunks while handling network transitions gracefully. This is NOT a typical WebRTC media use case - we want reliability over low latency.

2. **Debug logging is critical** - The 8MB rolling log file is not optional. Every chunk must be logged immediately. This is the user's safety net and personal archive.

3. **Chunked streaming for display only** - Chunks exist purely for real-time visual feedback. The complete text is always preserved in memory and debug log.

4. **WebSockets are only for signaling** - Used to establish the WebRTC connection, then DataChannels handle all data transfer. This separation provides connection resilience.

5. **Post-processing UI is preview only in V1** - Show users what's coming but keep it disabled. This sets expectations and avoids scope creep.

### Why Debug Logging Matters
The debug log isn't just for debugging - it's a feature. Users doing long dictation sessions need confidence their words are saved. The log provides:
- Instant recovery if the UI crashes
- Searchable history of all transcriptions  
- Peace of mind that nothing is lost
- Raw material for future processing experiments

The separation between streaming transcription (V1) and post-processing (V2) is intentional. Ship V1 when streaming is rock-solid, then layer on Claude integration.

Focus on making the happy path extremely fast and reliable. Edge cases can be handled with graceful degradation rather than complex recovery logic.

Remember: This replaces keyboard input for many workflows, so reliability and speed are paramount. The reliable DataChannel + debug log ensures users never lose a word, even on flaky connections.
