# Whisper.cpp and RNNoise Installation Setup

**Last Updated**: 2025-11-05
**Phase**: 2 Preparation

## Overview
Repeatable installation scripts for Phase 2 dependencies: Whisper.cpp for transcription and RNNoise for audio preprocessing.

## Whisper.cpp

### Official Sources
- **Repository**: https://github.com/ggml-org/whisper.cpp
- **Go bindings**: github.com/ggerganov/whisper.cpp/bindings/go/pkg/whisper
- **Models**: https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-{model}.bin

### Recommended Model
**large-v3-turbo**
- Size: ~1.6GB
- Performance: Fast + accurate balance
- Best for real-time transcription

### Platform-Specific Installation

#### macOS
Use Homebrew for Metal acceleration (40x faster):
```bash
brew install whisper-cpp
```

#### Linux
Build from source with CMake:
```bash
./scripts/install-whisper.sh
```

### Required CGO Flags
```bash
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build -lwhisper"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
```

**Important**: Must source `setup-env.sh` before building with whisper.cpp integration.

## RNNoise

### Package Information
- **Go package**: github.com/xaionaro-go/audio/pkg/noisesuppression/implementations/rnnoise
- **Package date**: April 2025
- **Frame size**: 10ms (160 samples at 16kHz)

### Pre-trained Model
- **Model name**: "leavened-quisling"
- **Source**: GregorR/rnnoise-models
- **URL**: https://github.com/GregorR/rnnoise-models/raw/refs/heads/master/leavened-quisling-2018-08-31/lq.rnnn

### Installation
```bash
./scripts/download-rnnoise.sh
```

## Installation Scripts

The project provides four setup scripts:

### 1. `scripts/install-whisper.sh`
- Builds `libwhisper.a` from source
- Compiles with optimization flags
- Creates necessary include directories

### 2. `scripts/download-models.sh`
- Downloads GGML models from Hugging Face
- Defaults to large-v3-turbo
- Validates downloads

### 3. `scripts/download-rnnoise.sh`
- Downloads RNNoise pre-trained model
- Fetches leavened-quisling model
- Places in appropriate directory

### 4. `scripts/setup-env.sh`
- Sets CGO environment variables
- Configures include paths
- Enables required compiler flags

## Build Process

### Complete Setup
```bash
# 1. Install Whisper.cpp
./scripts/install-whisper.sh

# 2. Download models
./scripts/download-models.sh

# 3. Download RNNoise model
./scripts/download-rnnoise.sh

# 4. Setup environment (must be sourced)
source ./scripts/setup-env.sh

# 5. Build project
go build ./...
```

### Verification
After setup, verify:
- `libwhisper.a` exists in whisper.cpp build directory
- GGML model file downloaded (~1.6GB)
- RNNoise model file present
- CGO flags set correctly

## Documentation Files

Additional setup documentation:
- `docs/SETUP.md` - General setup instructions
- `docs/PHASE2-PREP.md` - Phase 2 specific preparation

## Important Notes

1. **CGO Compilation**: Whisper.cpp requires C++ compilation, so CGO must be enabled
2. **Model Size**: Large-v3-turbo is 1.6GB - ensure sufficient disk space
3. **Environment Variables**: Must source `setup-env.sh` in each new shell session
4. **Metal Acceleration**: macOS users get significant speedup with Metal support
5. **Build Time**: Initial whisper.cpp build can take 5-10 minutes

## Troubleshooting

### Common Issues
- **Missing CGO flags**: Source `setup-env.sh`
- **Compilation errors**: Check CGO_CFLAGS_ALLOW includes `-mfma` and `-mf16c`
- **Model download fails**: Check internet connection and Hugging Face availability
- **Library not found**: Verify `WHISPER_DIR` environment variable is set correctly
