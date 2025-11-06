# Building with CGO Dependencies

Critical workflows for building the server with Whisper.cpp and RNNoise CGO dependencies.

---

## Always Test Builds Before Committing

**Problem**: Committing code without testing builds leads to compilation errors on user's machine.

**Solution**: ALWAYS test builds with CGO flags before committing.

### Build Test Command (Linux Container)

```bash
cd /workspace/project/server
export WHISPER_DIR=/workspace/project/deps/whisper.cpp
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build/src -L$WHISPER_DIR/build/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++ -lm"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"

# Test transcription package
go build ./internal/transcription/...

# Test full server
go build ./cmd/server
```

### Workflow
1. Make code changes
2. Run build test with CGO flags
3. Fix any compilation errors
4. ONLY THEN commit

### Common Build Errors to Check
- Unused imports (e.g., `"log" imported and not used`)
- Unused variables (e.g., `declared and not used: silenceDuration`)
- Type mismatches
- Missing dependencies

**Note**: Even when working in Linux container without Mac environment, CGO builds still work and catch compilation errors that would appear on Mac.

**Related**: Established 2025-11-06 during Session 8

---

## Building Server with RNNoise on Linux

**Status**: Fully working in Linux container

### Install RNNoise Library

Run the install script:
```bash
./scripts/install-rnnoise-lib.sh
```

This installs RNNoise to `/workspace/project/deps/rnnoise/`

### Build Server with RNNoise

Use these CGO environment variables:

```bash
export WHISPER_DIR=/workspace/project/deps/whisper.cpp
export RNNOISE_DIR=/workspace/project/deps/rnnoise
export PKG_CONFIG_PATH="$RNNOISE_DIR/lib/pkgconfig:$PKG_CONFIG_PATH"
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include -I$RNNOISE_DIR/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build/src -L$WHISPER_DIR/build/ggml/src -L$RNNOISE_DIR/lib -lwhisper -lggml -lggml-base -lggml-cpu -lrnnoise -lstdc++ -lm"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
export LD_LIBRARY_PATH="$RNNOISE_DIR/lib:$LD_LIBRARY_PATH"

# Build with RNNoise enabled
go build -tags rnnoise -o cmd/server/server ./cmd/server
```

### Build Without RNNoise (Pass-through)

If you don't need noise suppression:

```bash
# Just need Whisper env vars
export WHISPER_DIR=/workspace/project/deps/whisper.cpp
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build/src -L$WHISPER_DIR/build/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++ -lm"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"

# Build without -tags rnnoise (uses pass-through)
go build -o cmd/server/server ./cmd/server
```

### File Selection by Build Tag

- **Without `-tags rnnoise`**: Uses `rnnoise.go` (pass-through, no denoising)
- **With `-tags rnnoise`**: Uses `rnnoise_real.go` (real RNNoise with 16kHz↔48kHz conversion)

### Binary Size

- **With RNNoise**: ~17MB
- **Without RNNoise**: ~16MB (minimal difference)

### Running the Server

Remember to set `LD_LIBRARY_PATH` when running:

```bash
export LD_LIBRARY_PATH="/workspace/project/deps/rnnoise/lib:$LD_LIBRARY_PATH"
./cmd/server/server
```

Or use a wrapper script that sets the library path.

**Files**:
- `scripts/install-rnnoise-lib.sh`
- `server/internal/transcription/rnnoise.go` (pass-through)
- `server/internal/transcription/rnnoise_real.go` (real implementation)

**Related**: Established 2025-11-06 during RNNoise integration

---

## macOS Build Script with Auto-Detection

The `./scripts/build-mac.sh` script automatically detects locally-built RNNoise and Whisper.cpp:

### Auto-Detection Features
- Checks for `deps/rnnoise/lib/librnnoise.a`
- Checks for `deps/whisper.cpp/build/src/libwhisper.a`
- Sets `PKG_CONFIG_PATH` for locally-built rnnoise
- Adds `-tags rnnoise` if library found
- Configures CGO flags automatically

### Usage
```bash
./scripts/build-mac.sh
```

### What It Does
1. Detects available libraries
2. Sets environment variables
3. Builds with appropriate tags
4. Reports what was enabled

### Environment Variables Set
- `WHISPER_DIR` - Path to whisper.cpp
- `RNNOISE_DIR` - Path to rnnoise (if built)
- `PKG_CONFIG_PATH` - For rnnoise pkg-config
- `CGO_CFLAGS` - Include paths
- `CGO_LDFLAGS` - Library paths and linking
- `CGO_CFLAGS_ALLOW` - Allow compiler-specific flags

**Related**: Updated 2025-11-06 for RNNoise auto-detection

---
