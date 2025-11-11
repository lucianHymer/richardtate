# Config Hot-Reload Architecture

**Last Updated**: 2025-11-06 (Session 16)

## Overview
Automatic configuration reloading system for the client daemon that allows calibration changes to take effect immediately without requiring daemon restart.

## Problem Statement
Previously, after running calibration and saving a new VAD threshold, users had to restart the client daemon to pick up the new configuration values. This was inconvenient and broke the flow of the calibration workflow.

## Solution Architecture

### Config Reload Method
The Config struct now has a `Reload()` method that reloads configuration from disk and updates all fields in-place:

```go
func (c *Config) Reload() error {
    data, err := os.ReadFile(c.filePath)
    if err != nil {
        return err
    }

    // Parse and update fields in-place
    if err := yaml.Unmarshal(data, c); err != nil {
        return err
    }

    // Apply defaults for any missing fields
    c.setDefaults()

    return nil
}
```

### In-Place Update Pattern
**Key Design**: The config is updated in-place rather than creating a new struct. This ensures all components that hold references to the config automatically see the new values.

**Why This Works**:
- All components (WebRTC client, API server) hold a pointer to the same Config struct
- Updating fields in-place means all references see the new values immediately
- `SendControlStart()` reads from `c.config.Transcription.VAD.EnergyThreshold` each time it's called
- Next recording session automatically uses the new threshold

### Integration with Calibration

The calibration save endpoint (`POST /api/calibrate/save`) now:
1. Saves the threshold to the config file
2. Calls `cfg.Reload()` to reload the configuration
3. Logs confirmation: "Config reloaded - new threshold will be used on next recording: X.X"
4. Returns success to the caller

### Error Handling
If reload fails after successful save:
- Endpoint returns HTTP 500
- Message: "Config saved but reload failed: [error details]"
- The file is still saved, so a manual restart would pick up the changes

## User Workflow

### Before (Manual Restart Required)
1. Client daemon running
2. Run calibration (Hammerspoon or CLI)
3. Save threshold to config
4. **Stop client daemon**
5. **Start client daemon again**
6. Start new recording with new threshold

### Now (Automatic Hot-Reload)
1. Client daemon running
2. Run calibration (Hammerspoon or CLI)
3. Save threshold
4. Config automatically reloads (logged confirmation)
5. Start new recording → uses new threshold immediately
6. **NO RESTART REQUIRED**

## Implementation Details

### Config Storage
The Config struct stores its source file path:
```go
type Config struct {
    filePath string  // Path to the config file for reloading
    // ... other fields
}
```

### Load vs Reload
- `LoadConfig(path)` - Initial load, creates new Config struct
- `Config.Reload()` - Reload from same path, updates in-place

### Thread Safety
The current implementation does not use mutex protection for config access. This is acceptable because:
- Config reload happens infrequently (only on calibration save)
- Config fields are read at the start of operations (e.g., when starting recording)
- No continuous reading during operations

If concurrent access becomes an issue in the future, a `sync.RWMutex` could be added.

## Benefits

### Improved User Experience
- Seamless calibration workflow
- No interruption to daemon operation
- Immediate feedback on calibration changes

### Operational Benefits
- Less downtime
- Fewer manual steps
- Reduced chance of forgetting to restart

### Developer Benefits
- Clear separation between initial load and reload
- In-place update pattern preserves references
- Simple, maintainable implementation

## Testing
The hot-reload has been tested with the following scenario:
1. Start client daemon with initial threshold
2. Run Hammerspoon calibration wizard
3. Save new threshold
4. Verify log shows "Config reloaded - new threshold will be used on next recording"
5. Start new recording
6. Confirm new threshold is being used (visible in control.start message)

## Future Enhancements

### File Watch for Auto-Reload
Could implement file watching to automatically reload config when file changes:
```go
// Potential future enhancement
watcher, _ := fsnotify.NewWatcher()
watcher.Add(configPath)
go func() {
    for event := range watcher.Events {
        if event.Op&fsnotify.Write == fsnotify.Write {
            cfg.Reload()
        }
    }
}()
```

### Reload Other Settings
Currently focused on VAD threshold, but the pattern could extend to:
- Debug log settings
- Server endpoints
- Audio device configuration

### Validation on Reload
Could add validation to ensure reloaded config is valid before applying changes.

## Related Documentation
- [VAD Calibration API](vad-calibration-api.md) - The calibration system that triggers reloads
- [Hammerspoon Integration](hammerspoon-integration.md) - UI that saves calibration settings

## Files
- `client/internal/config/config.go` - Config struct with Reload() method
- `client/internal/api/server.go` - Calibration save endpoint that triggers reload