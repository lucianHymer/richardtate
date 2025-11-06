# Hammerspoon Integration Changelog

## [1.0.0] - 2025-11-06 - V1 Complete! 🎉

### Added
- **Hotkey control**: Ctrl+N to start/stop recording
- **Direct text insertion**: Text appears live at cursor position using `hs.eventtap.keyStrokes()`
- **Minimal visual indicator**: Small floating window shows "🔴 Recording..." in top-right corner
- **WebSocket integration**: Receives transcription chunks from client daemon in real-time
- **HTTP API integration**: Controls client daemon via `/start` and `/stop` endpoints
- **Automatic debug logging**: All transcriptions saved to `~/.config/richardtate/debug.log`
- **Installation script**: `install.sh` for easy setup with backup handling
- **Comprehensive documentation**: README with setup, usage, and troubleshooting

### Features
- Works in any application (Obsidian, VSCode, Slack, browser, etc.)
- Real-time streaming transcription (1-3 second latency)
- Clean, minimal UI (just a recording indicator)
- Configurable hotkey and daemon URL
- Reload config hotkey (Cmd+Alt+Ctrl+R)

### Technical Details
- Uses Hammerspoon canvas API for indicator UI
- Asynchronous HTTP requests for daemon control
- WebSocket client for receiving transcription chunks
- JSON message parsing for chunk and final transcription handling
- Text insertion via macOS key event simulation

### Known Limitations
- Requires macOS accessibility permissions
- Some security-focused apps may block text insertion
- Text insertion speed limited by macOS event simulation

## Future (V2)
- Post-processing modes (casual, professional, email, etc.)
- Preview UI before insertion
- Clipboard fallback for restricted apps
- Pause/resume during recording
- Audio level indicator
- Mode switching via keyboard shortcuts
