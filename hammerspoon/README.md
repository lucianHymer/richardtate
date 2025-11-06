# Hammerspoon Integration for Streaming Transcription

Simple hotkey-triggered voice transcription with direct text insertion.

## Features

- **Hotkey**: Press `Ctrl+N` to start/stop recording
- **Live transcription**: Text appears directly at your cursor as you speak
- **Visual indicator**: Small floating window shows recording status
- **Works everywhere**: Any text field in any app (Obsidian, VSCode, Slack, browser, etc.)
- **Automatic saving**: All transcriptions saved to debug log (`~/.streaming-transcription/debug.log`)
- **Calibration UI**: Press `Ctrl+Alt+C` to open visual VAD calibration wizard

## Prerequisites

1. **Hammerspoon**: Download from [https://www.hammerspoon.org/](https://www.hammerspoon.org/)
2. **Client daemon**: Must be running on `localhost:8081`
3. **Server**: Must be running and configured

## Installation

### 1. Install Hammerspoon

```bash
brew install --cask hammerspoon
```

### 2. Copy Scripts to Hammerspoon Config

```bash
# Create Hammerspoon config directory if it doesn't exist
mkdir -p ~/.hammerspoon

# Copy the scripts
cp /path/to/streaming-transcription/hammerspoon/init.lua ~/.hammerspoon/init.lua
cp /path/to/streaming-transcription/hammerspoon/calibration.lua ~/.hammerspoon/calibration.lua
```

**OR** symlink them (recommended for development):

```bash
ln -sf /path/to/streaming-transcription/hammerspoon/init.lua ~/.hammerspoon/init.lua
ln -sf /path/to/streaming-transcription/hammerspoon/calibration.lua ~/.hammerspoon/calibration.lua
```

### 3. Grant Accessibility Permissions

Hammerspoon needs accessibility permissions to insert text:

1. Open **System Preferences** → **Security & Privacy** → **Privacy**
2. Select **Accessibility** from the left sidebar
3. Click the lock icon to make changes
4. Add **Hammerspoon** to the list and enable it

### 4. Reload Hammerspoon

- Launch Hammerspoon (should show in menu bar)
- Click Hammerspoon menu → **Reload Config** (or press `Cmd+Alt+Ctrl+R`)

You should see a notification: "Streaming Transcription - Ctrl+N: Record | Ctrl+Alt+C: Calibrate"

## Usage

### Basic Workflow

1. **Start the client daemon**:
   ```bash
   cd /path/to/streaming-transcription/client
   ./client
   ```

2. **Click in any text field** (Obsidian, VSCode, browser, etc.)

3. **Press `Ctrl+N`** to start recording
   - Small indicator appears in top-right corner: "🔴 Recording..."

4. **Speak** into your microphone
   - Text appears live at your cursor as you speak
   - Chunks arrive in 1-3 second intervals

5. **Press `Ctrl+N` again** to stop recording
   - Indicator disappears
   - Final transcription saved to debug log

### VAD Calibration

**Important**: Before first use, calibrate Voice Activity Detection for your microphone:

1. **Press `Ctrl+Alt+C`** to open the calibration wizard

2. **Step 1 - Background Noise**:
   - Click "Start Recording"
   - Stay completely silent for 5 seconds
   - Measures ambient noise in your environment

3. **Step 2 - Speech**:
   - Click "Start Recording"
   - Speak normally and continuously for 5 seconds
   - Content doesn't matter, just speak naturally

4. **Step 3 - Results**:
   - View visual comparison of background vs speech
   - See recommended threshold
   - Click "Save & Close" to apply

5. **Done!** New threshold will be used on next recording session

**When to recalibrate**:
- Changed microphone
- Moved to different environment
- Noticing false positives (noise triggers transcription)
- Noticing false negatives (speech not detected)

### Keyboard Shortcuts

- `Ctrl+N` - Toggle recording (start/stop)
- `Ctrl+Alt+C` - Open VAD calibration wizard
- `Cmd+Alt+Ctrl+R` - Reload Hammerspoon config

## Configuration

Edit the `config` table at the top of `init.lua`:

```lua
local config = {
    daemonURL = "http://localhost:8081",  -- Client daemon URL
    wsURL = "ws://localhost:8081/transcriptions",  -- WebSocket endpoint
    hotkey = {mods = {"ctrl"}, key = "n"},  -- Change hotkey if desired
}
```

### Changing the Hotkey

Example: Change to `Cmd+Shift+V`:

```lua
hotkey = {mods = {"cmd", "shift"}, key = "v"},
```

### Using a Different Port

If your client runs on a different port:

```lua
daemonURL = "http://localhost:9000",
wsURL = "ws://localhost:9000/transcriptions",
```

## Troubleshooting

### Text Not Inserting

**Problem**: Text doesn't appear when speaking.

**Solutions**:
1. Check Hammerspoon has Accessibility permissions (see Installation step 3)
2. Verify client daemon is running: `curl http://localhost:8081/health`
3. Check Hammerspoon console for errors: Hammerspoon menu → **Console**

### WebSocket Connection Fails

**Problem**: Console shows "WebSocket failed" or "WebSocket closed"

**Solutions**:
1. Verify client daemon is running
2. Check client logs for WebSocket errors
3. Try reloading Hammerspoon config (`Cmd+Alt+Ctrl+R`)

### Hotkey Not Working

**Problem**: Pressing `Ctrl+N` doesn't start recording

**Solutions**:
1. Check for hotkey conflicts with other apps
2. Reload Hammerspoon config
3. Check Hammerspoon console for binding errors

### Recording Indicator Doesn't Appear

**Problem**: No visual indicator when pressing `Ctrl+N`

**Solutions**:
1. Check Hammerspoon console for canvas errors
2. Verify screen positioning (indicator is top-right by default)
3. Try different screen if using multiple monitors

## Debug Logging

All transcriptions are automatically saved to:

```
~/.streaming-transcription/debug.log
```

View recent transcriptions:

```bash
# View last 10 chunks
jq 'select(.type=="chunk")' ~/.streaming-transcription/debug.log | tail -10

# Get last complete session
jq -r 'select(.type=="complete") | .full_text' ~/.streaming-transcription/debug.log | tail -1

# Search for keyword
jq -r 'select(.text | contains("important"))' ~/.streaming-transcription/debug.log
```

## How It Works

1. **Hotkey pressed** → Hammerspoon calls `/start` API on client daemon
2. **Client daemon** → Starts audio capture, connects to server via WebRTC
3. **Server** → Processes audio (RNNoise → VAD → Smart Chunker → Whisper)
4. **Transcription chunks** → Sent back to client via DataChannel
5. **Client** → Forwards chunks to Hammerspoon via WebSocket (`/transcriptions`)
6. **Hammerspoon** → Inserts text at cursor using `hs.eventtap.keyStrokes()`

## Limitations

- **Text insertion speed**: Limited by macOS key event simulation (usually fine)
- **Some apps may block**: Security-focused apps might prevent text insertion
- **Clipboard fallback**: Not implemented yet (coming in V2)

## Next Steps (V2)

Future enhancements:
- Post-processing modes (casual, professional, Obsidian, code, email)
- Preview UI before insertion
- Clipboard fallback for restricted apps
- Pause/resume during recording
- Audio level indicator

## Support

For issues or questions:
- Check debug log: `~/.streaming-transcription/debug.log`
- Check Hammerspoon console: Hammerspoon menu → **Console**
- Review client logs for errors

## License

MIT License (same as parent project)
