# VAD Calibration Workflow

**Last Updated**: 2025-11-06 (Session 13)

## Overview
Interactive wizard for calibrating Voice Activity Detection (VAD) energy threshold. Helps users find optimal threshold for their microphone and environment by analyzing background noise vs speech energy.

## Purpose
- Eliminate false positives (noise triggering transcription)
- Eliminate false negatives (speech not detected)
- Adapt to different microphones and environments
- Provide data-driven threshold recommendations

## Command Usage

### Interactive Mode
```bash
./client --calibrate
```

**Behavior**:
- Records 5 seconds of background noise
- Records 5 seconds of speech
- Displays energy statistics and recommended threshold
- Prompts user to save to config

### Auto-Save Mode
```bash
./client --calibrate --yes
```

**Behavior**:
- Same calibration process
- Automatically accepts recommended threshold
- Saves to config without prompting

### Custom Config Path
```bash
./client --calibrate --config=/path/to/config.yaml
```

**Behavior**:
- Uses specified config file instead of default
- Useful for multiple profiles

## Calibration Process

### Step 1: Background Recording
```
VAD Calibration Wizard
======================

This will help you find the optimal VAD energy threshold for your environment.

Step 1: Record background noise (stay silent)

Recording background noise for 5 seconds...
[========================================] 5.0s
✓ Background recording complete!
```

**Instructions**:
- Stay completely silent
- Don't move or touch anything
- Let it capture ambient noise (fans, AC, street noise, etc.)

### Step 2: Speech Recording
```
Step 2: Record speech (speak normally)

Recording speech for 5 seconds...
[========================================] 5.0s
✓ Speech recording complete!
```

**Instructions**:
- Speak naturally and continuously
- Use normal speaking volume
- Say random sentences (doesn't matter what)

### Step 3: Analysis Display
```
Analysis Results:
==================

Background Noise:
  Min:  12.3
  Max:  89.4
  Avg:  45.2
  P95:  78.1  ← 95th percentile

Speech:
  Min:  234.5
  Max:  1823.7
  Avg:  654.3
  P5:   290.2  ← 5th percentile

Visual Comparison:
------------------
Background P95: ████████ 78.1
Speech P5:      ████████████████████████████████ 290.2

Recommended threshold: 184.2
(Calculated as: (background_p95 + speech_p5) / 2)

Save this threshold to config? [y/N]:
```

### Step 4: Save Decision

**If user enters 'y'**:
```
Manual Config Update Required:
-------------------------------

Please add this line to your config file:

vad:
  energy_threshold: 184.2

Config file location: /home/user/.voice-notes/config.yaml

✓ Calibration complete!
```

**If user enters 'N' or anything else**:
```
Threshold not saved. You can manually add it to your config if needed.
```

## How It Works

### Architecture

**Server-Side Analysis** (not client-side):
- Client captures audio samples
- Client sends samples to server `/api/v1/analyze-audio`
- Server calculates energy using SAME VAD algorithm as production
- Server returns statistics
- Client calculates recommended threshold and displays results

**Why Server-Side**:
1. VAD implementation lives in server
2. Guarantees calibration uses exact same energy calculation as production
3. Avoids code duplication between client/server
4. Keeps client lightweight

### Energy Calculation

**Frame-Based Analysis** (10ms frames):
```go
// For each 10ms frame (160 samples at 16kHz)
var sum float64
for _, sample := range frame {
    sum += float64(sample * sample)
}
rmsEnergy := math.Sqrt(sum / float64(len(frame)))
```

**Same as Production VAD**: This is identical to the energy calculation used by the VAD in the transcription pipeline.

### Statistical Analysis

**Background Statistics**:
- Min, Max, Avg: Range of background noise energy
- **P95** (95th percentile): High end of background noise (excludes outliers)

**Speech Statistics**:
- Min, Max, Avg: Range of speech energy
- **P5** (5th percentile): Low end of speech energy (quiet speaking)

**Threshold Calculation**:
```
recommended_threshold = (background_p95 + speech_p5) / 2
```

**Why This Works**:
- Background P95: Captures even noisy background (AC, fans, etc.)
- Speech P5: Captures even quiet speech
- Midpoint: Maximizes separation between noise and speech

### API Contract

**Endpoint**: `POST /api/v1/analyze-audio`

**Request**:
```json
{
  "audio": [/* byte array of PCM int16 samples */]
}
```

**Response**:
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

## Usage Tips

### When to Calibrate

**Mandatory**:
- First time setup
- Changed microphone
- Moved to different environment

**Optional**:
- Noticing false positives (noise triggers transcription)
- Noticing false negatives (speech not detected)
- Periodically (every few months)

### Getting Good Results

**Background Recording**:
- Do NOT mute microphone (captures silence, not noise!)
- Stay in normal recording position
- Capture typical ambient noise (fans, AC, street noise)
- Don't be artificially quiet (normal environment)

**Speech Recording**:
- Speak naturally (not shouting, not whispering)
- Continuous speech (don't pause for 5 seconds)
- Normal volume for your use case
- Representative of how you'll actually use it

### Interpreting Results

**Good Separation** (recommended threshold between P95 and P5):
```
Background P95: ████████ 78.1
Speech P5:      ████████████████████████████████ 290.2
Threshold:      ████████████ 184.2  ✓ Good!
```

**Poor Separation** (overlapping ranges):
```
Background P95: ████████████████████ 450.3
Speech P5:      ████████████ 320.1
Threshold:      ██████████████ 385.2  ⚠️ May have issues
```

**If Poor Separation**:
- Try quieter environment
- Move closer to microphone
- Use different microphone
- Consider noise-canceling microphone

## Known Limitations

### RNNoise Processing Missing

**CRITICAL ISSUE** (not yet fixed):
- Calibration analyzes RAW audio
- Production pipeline uses RNNoise-processed audio
- RNNoise reduces background noise by 30-50%
- Calibration may recommend thresholds that are too high

**Workaround**:
If using RNNoise in production (`-tags rnnoise`), manually reduce recommended threshold by ~30%:

```
Recommended: 184.2
Use instead: 129.0  (184.2 * 0.7)
```

**Future Fix**:
Add RNNoise processing to calibration endpoint so it matches production exactly.

### Manual Config Update

**Current Limitation**: Calibration displays recommended threshold but doesn't automatically update config file.

**Why**: YAML parsing/updating is complex. Safer to have user manually add it.

**Future Enhancement**: Could add automatic config update with YAML parser.

## Implementation Details

### Client Components

**Package**: `client/internal/calibrate/`

**Main Function**: `calibrate.Run(configPath, autoYes)`

**Flow**:
1. Initialize audio capture
2. Record background (5s)
3. Send to server, get stats
4. Record speech (5s)
5. Send to server, get stats
6. Calculate recommended threshold
7. Display results with visual bars
8. Prompt to save (or auto-save if `--yes`)
9. Display manual instructions

**Terminal UI**:
- Progress bars during recording
- Visual bar chart comparison
- Interactive yes/no prompt
- Color-coded output

### Server Components

**Endpoint**: `/api/v1/analyze-audio` in `server/internal/api/server.go`

**Handler**: `handleAnalyzeAudio()`

**Processing**:
1. Receive audio bytes
2. Convert to int16 samples
3. Split into 10ms frames (160 samples at 16kHz)
4. Calculate RMS energy per frame
5. Compute min, max, avg, p5, p95 statistics
6. Return JSON response

### Statistics Calculation

**Percentile Calculation**:
```go
func percentile(sorted []float64, p float64) float64 {
    idx := int(float64(len(sorted)-1) * p)
    return sorted[idx]
}
```

**Requires**: Sorted energy values

## Related Systems

### VAD System
Calibration directly affects VAD behavior:
- Threshold set in config
- Used by VAD to detect speech vs silence
- See [Transcription Pipeline](../architecture/transcription-pipeline.md)

### Audio Capture
Reuses existing audio capture infrastructure:
- Same device selection
- Same sample rate (16kHz)
- Same format (mono PCM int16)

## Troubleshooting

### "Connection refused" Error
**Cause**: Server not running

**Fix**: Start server first:
```bash
./server --config server-config.yaml
```

### Negative or Zero Threshold
**Cause**: Very quiet background or very loud noise floor

**Fix**: Check microphone levels, try different environment

### Threshold Seems Too High/Low
**Cause**: RNNoise processing difference (see Known Limitations)

**Fix**: Manually adjust by ~30% if using RNNoise in production

## Future Enhancements

1. **RNNoise integration**: Process audio through RNNoise during calibration
2. **Automatic config update**: Parse and update YAML automatically
3. **Multi-environment profiles**: Save different thresholds for different locations
4. **Calibration history**: Track threshold changes over time
5. **Validation mode**: Test current threshold before saving

## Related Files

**Client Implementation**:
- `client/internal/calibrate/calibrate.go` - Main wizard logic
- `client/cmd/client/main.go` - Flag handling and integration

**Server Implementation**:
- `server/internal/api/server.go:handleAnalyzeAudio()` - Energy analysis endpoint

**Related Documentation**:
- [Transcription Pipeline](../architecture/transcription-pipeline.md) - VAD system details
- [Gotchas: VAD Calibration Missing RNNoise](../gotchas/transcription-gotchas.md#vad-calibration-missing-rnnoise-processing) - Known issue
