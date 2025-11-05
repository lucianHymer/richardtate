# Setup Guide

This guide will help you set up the development environment for the Streaming Transcription System.

## Prerequisites

- **Go 1.21+** with CGO enabled
- **CMake 3.10+** for building whisper.cpp
- **C++ compiler** (g++, clang++)
- **curl** or **wget** for downloading models
- **ALSA development libraries** (Linux) for audio capture:
  ```bash
  # Fedora/RHEL
  sudo dnf install alsa-lib-devel

  # Ubuntu/Debian
  sudo apt-get install libasound2-dev
  ```

## Platform-Specific Setup

### macOS (Recommended for Apple Silicon)

If you're on macOS with Apple Silicon, use Homebrew for easier setup with Metal acceleration:

```bash
# Install whisper.cpp with Metal acceleration
brew install whisper-cpp

# Download the large-v3-turbo model
mkdir -p ~/.cache/whisper
curl -L -o ~/.cache/whisper/ggml-large-v3-turbo.bin \
  "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin?download=true"

# Install audio tools (optional, for future features)
brew install sox ffmpeg

# Download RNNoise model for noise suppression
./scripts/download-rnnoise.sh
```

**Benefits:**
- ✅ Metal GPU acceleration (40x faster than CPU)
- ✅ Automatic updates via Homebrew
- ✅ No manual compilation needed

After this, skip to step 4 (Install Go Dependencies).

### Linux / Manual Build

## Quick Start

### 1. Install Whisper.cpp

This downloads and builds the whisper.cpp library:

```bash
./scripts/install-whisper.sh
```

This will:
- Clone whisper.cpp to `deps/whisper.cpp`
- Build `libwhisper.a` static library
- Set up the directory structure

**Expected time:** 2-5 minutes depending on your CPU

### 2. Download Whisper Models

Download the large-v3-turbo model (~1.6GB):

```bash
./scripts/download-models.sh
```

To download different models, edit `scripts/download-models.sh` and uncomment the models you want.

**Available models:**
- `tiny` / `tiny.en` - ~75MB, fast but less accurate
- `base` / `base.en` - ~142MB, good balance
- `small` / `small.en` - ~466MB, better accuracy
- `medium` / `medium.en` - ~1.5GB, high accuracy
- `large-v3-turbo` - ~1.6GB, **recommended** (fast + accurate)
- `large-v3` - ~3GB, best accuracy but slower

### 2.5. Download RNNoise Model (Phase 2)

Download the RNNoise noise suppression model:

```bash
./scripts/download-rnnoise.sh
```

This downloads the "leavened-quisling" model from GregorR's trained models repository. This model is used for real-time noise suppression before transcription.

### 3. Set Up Environment

Before building, source the environment setup script:

```bash
source ./scripts/setup-env.sh
```

This sets the required CGO environment variables:
- `CGO_CFLAGS` - Include paths for whisper.h
- `CGO_LDFLAGS` - Library paths for libwhisper.a
- `CGO_CFLAGS_ALLOW` - CPU optimization flags

**Note:** You need to run this in every new shell session before building.

### 4. Install Go Dependencies

```bash
make deps
```

This installs:
- Official whisper.cpp Go bindings
- RNNoise Go package (for noise suppression)
- All other Go dependencies

### 5. Build

```bash
make build
```

This builds both server and client binaries.

## Running the System

### Terminal 1: Start Server
```bash
./server/cmd/server/server
```

### Terminal 2: Start Client
```bash
./client/cmd/client/client
```

### Terminal 3: Test Recording
```bash
# Start recording
curl -X POST http://localhost:8081/start

# Stop after speaking
curl -X POST http://localhost:8081/stop
```

## Development Workflow

### Building with Environment Variables (Recommended)

Create a build script or add to your shell profile:

```bash
# build.sh
#!/bin/bash
source ./scripts/setup-env.sh
make build
```

### Alternative: Set Variables Permanently

Add to your `~/.bashrc` or `~/.zshrc`:

```bash
export WHISPER_DIR="/workspace/project/deps/whisper.cpp"
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build -lwhisper"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
export LIBRARY_PATH="$WHISPER_DIR/build:${LIBRARY_PATH}"
```

## Troubleshooting

### "libwhisper.a not found"

Make sure you ran `./scripts/install-whisper.sh` successfully.

Verify the library exists:
```bash
ls -lh deps/whisper.cpp/build/libwhisper.a
```

### "cannot find -lwhisper" during build

Run `source ./scripts/setup-env.sh` before building.

### "ggml-large-v3-turbo.bin not found" at runtime

Download models with:
```bash
./scripts/download-models.sh
```

Verify:
```bash
ls -lh models/
```

### Audio capture fails

On Linux, ensure ALSA development libraries are installed:
```bash
# Fedora/RHEL
sudo dnf install alsa-lib-devel

# Ubuntu/Debian
sudo apt-get install libasound2-dev
```

### CGO compilation errors

Make sure you have a C++ compiler:
```bash
# Check compiler
g++ --version
# or
clang++ --version
```

## Project Structure

```
/workspace/project/
├── server/          # Go server application
├── client/          # Go client daemon
├── shared/          # Shared protocol definitions
├── deps/            # External dependencies (gitignored)
│   └── whisper.cpp/ # Whisper.cpp source and build
├── models/          # Whisper GGML models (gitignored)
│   └── ggml-large-v3-turbo.bin
└── scripts/         # Installation and setup scripts
    ├── install-whisper.sh
    ├── download-models.sh
    └── setup-env.sh
```

## Next Steps

- Read [streaming-transcription-implementation-plan.md](../streaming-transcription-implementation-plan.md) for architecture details
- Check server config: `server/config.example.yaml`
- Check client config: `client/config.example.yaml`
- Review Phase 1 completion status in the implementation plan

## Clean Installation

To start fresh:

```bash
# Remove all dependencies and models
rm -rf deps/ models/

# Remove binaries
make clean

# Reinstall
./scripts/install-whisper.sh
./scripts/download-models.sh
source ./scripts/setup-env.sh
make build
```
