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

