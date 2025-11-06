# Richardtate

<p align="center">
    <i>
	Dictation for the discerning orator.
    </i>
    <br><br>
    <img
        src="https://raw.githubusercontent.com/lucianHymer/richardtate/refs/heads/main/assets/logo.png"
        width="320px"
        alt="logo"
    />
</p>

Real-time voice-to-text transcription that streams your spoken words directly into any application. Press a hotkey, speak naturally, and watch your words appear at your cursor with near-realtime feedback.

## Architecture

Richardtate is a three-component system that processes your voice locally with state-of-the-art ML models:

```
┌─────────────────┐
│   Hammerspoon   │  Hotkey control (Ctrl+N)
│   Lua Script    │  Text insertion at cursor
└────────┬────────┘  Visual recording indicator
         │
         │ HTTP API
         ▼
┌─────────────────┐
│  Client Daemon  │  Audio capture (16kHz mono)
│   (Go binary)   │  WebRTC streaming
└────────┬────────┘  Reconnection logic
         │
         │ WebRTC DataChannel
         │ (200ms chunks)
         ▼
┌─────────────────┐
│     Server      │  RNNoise → VAD → Whisper
│   (Go binary)   │  Streaming transcriptions
└─────────────────┘  Local ML inference
```

### The Audio Pipeline

The server implements a sophisticated real-time transcription pipeline:

```
Raw Audio (200ms chunks @ 16kHz)
  ↓
┌─────────────────────────┐
│  RNNoise Neural Net     │  Noise suppression
│  (48kHz processing)     │  + sample rate conversion
└───────────┬─────────────┘
            ↓
┌─────────────────────────┐
│  Voice Activity Detect  │  Speech vs silence
│  (energy-based)         │  Frame-level analysis
└───────────┬─────────────┘
            ↓
┌─────────────────────────┐
│  Smart Chunker          │  1s silence → transcribe
│  (VAD-driven)           │  Natural speech boundaries
└───────────┬─────────────┘
            ↓
┌─────────────────────────┐
│  Whisper.cpp            │  Speech-to-text
│  (large-v3-turbo)       │  Metal acceleration (Mac)
└───────────┬─────────────┘
            ↓
     Streaming Results
    (1-3 second latency)
```

### Key Technical Features

**WebRTC Reliability**
- Exponential backoff reconnection (1s → 30s max)
- 20-second audio buffer during disconnections
- 99% data integrity during server crashes
- Automatic connection recovery

**Audio Intelligence**
- RNNoise removes background noise (keyboards, fans, traffic)
- VAD chunks on natural speech pauses (no mid-sentence cuts)
- Minimum 1s speech requirement (prevents Whisper hallucinations)
- 16kHz ↔ 48kHz resampling for RNNoise compatibility

**Hammerspoon Integration**
- System-wide hotkey (Ctrl+N) for recording control
- Direct text insertion at cursor (works in any app!)
- Minimal visual indicator during recording
- HTTP API for client communication

**Local-First Privacy**
- All processing runs on your machine
- No cloud API calls
- Models loaded from disk
- Audio never leaves your computer

## How It Works

1. **Press Ctrl+N** in Hammerspoon → HTTP request to client daemon
2. **Client captures audio** → 16kHz mono PCM, 200ms chunks
3. **WebRTC streams to server** → Reliable DataChannel transport
4. **RNNoise cleans audio** → Neural noise suppression
5. **VAD detects speech** → Waits for 1s of silence
6. **Whisper transcribes** → Large-v3-turbo model (~1.6GB)
7. **Text streams back** → Client receives transcription
8. **Hammerspoon inserts** → Text appears at your cursor

Total latency: **1-3 seconds** from speech to text (on Apple Silicon with Metal acceleration).

## Tech Stack

**Languages**
- Go (client + server)
- Lua (Hammerspoon integration)

**ML Models**
- Whisper.cpp (large-v3-turbo) - Speech recognition
- RNNoise (leavened-quisling) - Noise suppression

**Communication**
- WebRTC DataChannels - Real-time audio streaming
- HTTP API - Hammerspoon ↔ Client
- JSON - Transcription results

**Platform**
- macOS (primary target, Metal acceleration)
- Hammerspoon - System automation framework
- CGO - Native library bindings

## Project Status

✅ **V1 Complete** - Real-time streaming transcription with direct text insertion!

**What Works:**
- Real-time streaming transcription
- RNNoise noise reduction
- Smart VAD-based chunking
- Direct text insertion (any app!)
- WebRTC reconnection + buffering
- Automatic debug logging
- Hotkey control (Ctrl+N)

**Coming in V2:**
- Smart text formatting modes
- Preview before insertion
- LLM-powered cleanup
- Markdown support

## Installation (macOS)

### Quick Start (One Command)

```bash
./scripts/install-mac.sh
```

This installs everything:
- Whisper.cpp with Metal acceleration
- Whisper large-v3-turbo model (~1.6GB)
- RNNoise library and model
- Creates `~/.config/richardtate/` directory
- Builds client and server binaries

### Manual Installation

If you prefer manual setup, see the detailed steps in [docs/SETUP.md](docs/SETUP.md).

### Post-Installation

1. **Calibrate VAD threshold** (one-time setup):
   ```bash
   cd client
   ./client --calibrate
   ```

   Follow the wizard to measure your microphone's background noise and speech levels.

2. **Start the services as background daemons:**
   ```bash
   richardtate start
   ```

   The client and server now run in the background! They will:
   - Auto-start on login
   - Auto-restart on crash
   - Log to `~/.config/richardtate/logs/`

3. **Control the services:**
   ```bash
   richardtate status   # Check if running
   richardtate logs     # View logs
   richardtate restart  # Restart both
   richardtate stop     # Stop both
   ```

   See [docs/DAEMON-SETUP.md](docs/DAEMON-SETUP.md) for details.

4. **(Optional) Install Hammerspoon integration:**
   ```bash
   brew install --cask hammerspoon
   cd hammerspoon
   ./install.sh
   ```

   Grant accessibility permissions, then reload Hammerspoon. Press **Ctrl+N** to start/stop recording.

### Configuration

- Client config: `~/.config/richardtate/client.yaml`
- Server config: `~/.config/richardtate/server.yaml`
- Debug logs: `~/.config/richardtate/debug.log`
- Example configs: `client/config.example.yaml` and `server/config.example.yaml`

