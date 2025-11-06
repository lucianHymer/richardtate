### [21:41] [gotcha] Calibration saved to wrong config path
**Details**: **Discovered**: 2025-11-06 (Session 16)

**Problem**: API calibration endpoint was saving threshold to wrong YAML path:
- Saving to: `transcription.vad_energy_threshold` (flat, wrong)
- Should be: `transcription.vad.energy_threshold` (nested, correct)

**Impact**: 
- Calibration appeared to succeed but threshold wasn't actually used
- Client code reads from nested structure only
- CLI calibration had correct implementation, API calibration had wrong path

**Root Cause**: Code duplication between CLI calibration and API calibration endpoints

**Solution**:
1. Created shared `config.UpdateVADThreshold()` function in `client/internal/config/update.go`
2. Both CLI calibration and API calibration now use same function
3. Ensures consistency and prevents future divergence

**Files Changed**:
- `client/internal/config/update.go` - New shared function
- `client/internal/api/server.go` - Use shared function
- `client/internal/calibrate/calibrate.go` - Use shared function
- `.claude/knowledge/architecture/vad-calibration-api.md` - Updated docs

**Lesson**: When two code paths do the same thing, extract to shared function immediately
**Files**: client/internal/config/update.go, client/internal/api/server.go, client/internal/calibrate/calibrate.go
---

### [15:03] [architecture] Config hot-reload on calibration save
**Details**: The calibration save endpoint now automatically reloads the client config after saving the new VAD threshold. This eliminates the need to restart the client daemon to pick up calibration changes.

**Implementation**:
- Added `Config.Reload()` method that reloads from disk and updates config in-place
- Config stores its file path (`filePath` field) for reloading
- Calibration save endpoint calls `cfg.Reload()` after successful save
- Updates all fields in-place to preserve references

**Why in-place update works**:
- All components (WebRTC client, API server) hold a pointer to the same config struct
- Updating fields in-place means all references see the new values immediately
- `SendControlStart()` reads from `c.config.Transcription.VAD.EnergyThreshold` each time
- Next recording session automatically uses the new threshold

**User workflow**:
1. Client daemon running
2. Run calibration (Hammerspoon or CLI)
3. Save threshold
4. Config auto-reloads (logged: "Config reloaded - new threshold will be used on next recording: X.X")
5. Start new recording → uses new threshold
6. NO RESTART REQUIRED

**Error handling**: If reload fails, endpoint returns 500 with "Config saved but reload failed" message.
**Files**: client/internal/config/config.go, client/internal/api/server.go
---

### [22:19] [gotcha] Default debug log path was read-only filesystem issue
**Details**: The client config default for debug_log_path was set to "./debug.log" (current directory) on lines 60 and 116 of config.go. When client ran in a read-only filesystem, it would FATAL on startup. Fixed to use "~/.config/richardtate/debug.log" to match config.example.yaml default. The debuglog package already handles ~ expansion correctly.
**Files**: client/internal/config/config.go
---

