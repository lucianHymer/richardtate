#!/bin/bash

# Environment Setup Script
# Source this file before building: source ./scripts/setup-env.sh

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
WHISPER_DIR="$PROJECT_ROOT/deps/whisper.cpp"

if [ ! -d "$WHISPER_DIR" ]; then
    echo "❌ Whisper.cpp not found. Run ./scripts/install-whisper.sh first"
    return 1
fi

if [ ! -f "$WHISPER_DIR/build/src/libwhisper.a" ]; then
    echo "❌ libwhisper.a not found. Run ./scripts/install-whisper.sh first"
    return 1
fi

# Set CGO environment variables for whisper.cpp
export CGO_CFLAGS="-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include"
export CGO_LDFLAGS="-L$WHISPER_DIR/build/src -L$WHISPER_DIR/build/ggml/src -lwhisper -lggml -lggml-base -lggml-cpu -lstdc++ -lm"
export CGO_CFLAGS_ALLOW="-mfma|-mf16c"
export LIBRARY_PATH="$WHISPER_DIR/build/src:$WHISPER_DIR/build/ggml/src:${LIBRARY_PATH}"
export LD_LIBRARY_PATH="$WHISPER_DIR/build/src:$WHISPER_DIR/build/ggml/src:${LD_LIBRARY_PATH}"

echo "✅ Environment configured for whisper.cpp"
echo "CGO_CFLAGS=$CGO_CFLAGS"
echo "CGO_LDFLAGS=$CGO_LDFLAGS"
echo ""
echo "You can now build the server with: make server"
