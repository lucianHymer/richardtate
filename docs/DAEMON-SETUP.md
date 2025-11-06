# Richard Tate Daemon Setup (macOS)

This document explains how the Richard Tate services run as background daemons on macOS using launchd.

## Overview

After running `scripts/install-mac.sh`, both the server and client are installed as **user launchd services** that:

- Run in the background automatically
- Start on login (via `RunAtLoad`)
- Auto-restart on crash (via `KeepAlive`)
- Log to `~/.config/richardtate/logs/`

## Control Commands

Use the `richardtate` command to manage the services:

```bash
# Start both services
richardtate start

# Stop both services
richardtate stop

# Restart both services
richardtate restart

# Check status
richardtate status

# View logs (all)
richardtate logs

# View specific logs
richardtate logs server
richardtate logs client
```

## Service Details

### Server Service
- **Label**: `com.richardtate.server`
- **Binary**: `<project>/server/server`
- **Config**: `~/.config/richardtate/server.yaml`
- **Log**: `~/.config/richardtate/logs/server.log`
- **Error Log**: `~/.config/richardtate/logs/server.err`

### Client Service
- **Label**: `com.richardtate.client`
- **Binary**: `<project>/client/client`
- **Config**: `~/.config/richardtate/client.yaml`
- **Log**: `~/.config/richardtate/logs/client.log`
- **Error Log**: `~/.config/richardtate/logs/client.err`

## Manual launchd Commands

If you need to interact with launchd directly:

```bash
# Load services (start + enable auto-start)
launchctl load ~/Library/LaunchAgents/com.richardtate.server.plist
launchctl load ~/Library/LaunchAgents/com.richardtate.client.plist

# Unload services (stop + disable auto-start)
launchctl unload ~/Library/LaunchAgents/com.richardtate.server.plist
launchctl unload ~/Library/LaunchAgents/com.richardtate.client.plist

# List running services
launchctl list | grep richardtate

# View service details
launchctl list com.richardtate.server
launchctl list com.richardtate.client
```

## Troubleshooting

### Services won't start

1. **Check if plists are installed:**
   ```bash
   ls ~/Library/LaunchAgents/com.richardtate.*
   ```

2. **Check if binaries exist:**
   ```bash
   ls <project>/server/server
   ls <project>/client/client
   ```

3. **Check logs for errors:**
   ```bash
   richardtate logs
   ```

4. **Try loading manually:**
   ```bash
   launchctl load ~/Library/LaunchAgents/com.richardtate.server.plist
   launchctl list | grep richardtate
   ```

### Services keep crashing

1. **Check error logs:**
   ```bash
   tail -f ~/.config/richardtate/logs/server.err
   tail -f ~/.config/richardtate/logs/client.err
   ```

2. **Check config files:**
   ```bash
   cat ~/.config/richardtate/server.yaml
   cat ~/.config/richardtate/client.yaml
   ```

3. **Try running manually to see errors:**
   ```bash
   cd <project>/server
   ./server --config ~/.config/richardtate/server.yaml

   # In another terminal
   cd <project>/client
   ./client --config ~/.config/richardtate/client.yaml
   ```

### Updating binaries

After rebuilding the client or server:

```bash
# Rebuild
cd <project>
scripts/build-mac.sh

# Restart services to use new binaries
richardtate restart
```

### Disabling auto-start

If you don't want services to start automatically on login:

```bash
# Unload the services (stops them and disables auto-start)
richardtate stop

# To start manually later:
richardtate start
```

### Removing services completely

```bash
# Stop and unload
richardtate stop

# Remove plists
rm ~/Library/LaunchAgents/com.richardtate.server.plist
rm ~/Library/LaunchAgents/com.richardtate.client.plist

# Remove control script
sudo rm /usr/local/bin/richardtate
```

## File Locations

### Installed Files
- **launchd plists**: `~/Library/LaunchAgents/com.richardtate.*.plist`
- **Control script**: `/usr/local/bin/richardtate`

### Runtime Files
- **Configs**: `~/.config/richardtate/client.yaml` and `server.yaml`
- **Logs**: `~/.config/richardtate/logs/`
- **Debug log**: `~/.config/richardtate/debug.log` (client only)

### Source Files
- **Template plists**: `<project>/scripts/com.richardtate.*.plist`
- **Control script source**: `<project>/scripts/richardtate`

## How It Works

### Installation Process

1. `install-mac.sh` builds the binaries
2. Template plists are processed with `sed` to replace:
   - `PROJECT_ROOT` → actual project path
   - `HOME` → user's home directory
3. Processed plists are installed to `~/Library/LaunchAgents/`
4. Control script is installed to `/usr/local/bin/richardtate`

### Service Lifecycle

1. **On login**: launchd reads `~/Library/LaunchAgents/*.plist`
2. **RunAtLoad=true**: Services start automatically
3. **KeepAlive=true**: Services restart if they crash
4. **StandardOutPath/StandardErrorPath**: Logs are captured

### Control Script

The `richardtate` script is a convenience wrapper that:
- Calls `launchctl load/unload` to start/stop services
- Checks service status via `launchctl list`
- Tails log files for debugging

## Integration with Hammerspoon

The Hammerspoon integration expects the **client daemon to be running** at `localhost:8081`. With the launchd setup:

1. Services start automatically on login
2. Hammerspoon can immediately connect (no manual startup needed)
3. Press **Ctrl+N** in any app to start/stop recording
4. Press **Ctrl+Alt+C** to run VAD calibration wizard

Perfect workflow:
- Log into macOS → services auto-start
- Install Hammerspoon → press Ctrl+N → it just works!

## Benefits Over Manual Startup

### Before (Manual)
```bash
# Terminal 1
cd /path/to/project/server
./server

# Terminal 2
cd /path/to/project/client
./client

# Keep both terminals open forever
# Re-run if crashed
# Re-run on every login
```

### After (Daemon)
```bash
# One-time setup
scripts/install-mac.sh

# Then forget about it - services run in background
# Auto-start on login
# Auto-restart on crash
# Control with simple commands:
richardtate status
richardtate logs
```

## Future Enhancements

Possible improvements:
- **systemd support**: Linux equivalent (for development)
- **Notifications**: macOS notifications on service start/stop/crash
- **Health checks**: Periodic ping to verify services are responding
- **Log rotation**: Automatic log file rotation (currently unbounded)
- **Update command**: `richardtate update` to pull latest and rebuild
