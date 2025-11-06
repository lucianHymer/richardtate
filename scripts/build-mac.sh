#!/bin/bash
#
# macOS Build Script for Streaming Transcription
#
# Prerequisites:
#   brew install whisper-cpp
#   brew install rnnoise
#   brew install go
#

set -e

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

echo "🍎 macOS Build Script"
echo "===================="
echo ""

# Check if Homebrew is installed
if ! command -v brew &> /dev/null; then
    echo "❌ Homebrew not found. Please install from https://brew.sh"
    exit 1
fi

# Check if whisper-cpp is installed
if ! brew list whisper-cpp &> /dev/null; then
    echo "❌ whisper-cpp not installed."
    echo ""
    echo "Install it with:"
    echo "  brew install whisper-cpp"
    echo ""
    exit 1
fi

# Check if rnnoise is installed (optional but recommended)
ENABLE_RNNOISE=false
if brew list rnnoise &> /dev/null 2>&1; then
    echo "✅ Found rnnoise - will build with noise suppression"
    ENABLE_RNNOISE=true
    RNNOISE_PREFIX=$(brew --prefix rnnoise)
else
    echo "⚠️  rnnoise not installed - building WITHOUT noise suppression"
    echo ""
    echo "To enable RNNoise (recommended for noisy environments):"
    echo "  brew install rnnoise"
    echo "  Then re-run this script"
    echo ""
fi

# Get whisper-cpp installation path
WHISPER_PREFIX=$(brew --prefix whisper-cpp)
echo "✅ Found whisper-cpp at: $WHISPER_PREFIX"

# Verify the include and lib directories exist
if [ ! -d "$WHISPER_PREFIX/libexec/include" ]; then
    echo "❌ Include directory not found at $WHISPER_PREFIX/libexec/include"
    exit 1
fi

if [ ! -d "$WHISPER_PREFIX/libexec/lib" ]; then
    echo "❌ Library directory not found at $WHISPER_PREFIX/libexec/lib"
    exit 1
fi

# Set CGO environment variables for Whisper
export CGO_CFLAGS="-I${WHISPER_PREFIX}/libexec/include"
export CGO_LDFLAGS="-L${WHISPER_PREFIX}/libexec/lib -lwhisper"

# Add RNNoise if available
BUILD_TAGS=""
if [ "$ENABLE_RNNOISE" = true ]; then
    export CGO_CFLAGS="$CGO_CFLAGS -I${RNNOISE_PREFIX}/include"
    export CGO_LDFLAGS="$CGO_LDFLAGS -L${RNNOISE_PREFIX}/lib -lrnnoise"
    BUILD_TAGS="-tags rnnoise"
fi

echo "✅ CGO environment configured"
echo "   CGO_CFLAGS=$CGO_CFLAGS"
echo "   CGO_LDFLAGS=$CGO_LDFLAGS"
if [ -n "$BUILD_TAGS" ]; then
    echo "   BUILD_TAGS=$BUILD_TAGS"
fi
echo ""

# Build server
echo "🔨 Building server..."
cd "$PROJECT_ROOT/server"
go build $BUILD_TAGS -o cmd/server/server ./cmd/server
SERVER_SIZE=$(du -h cmd/server/server | cut -f1)
echo "✅ Server built: server/cmd/server/server ($SERVER_SIZE)"
if [ "$ENABLE_RNNOISE" = true ]; then
    echo "   🎯 RNNoise enabled - noise suppression active!"
fi
echo ""

# Build client
echo "🔨 Building client..."
cd "$PROJECT_ROOT/client"
go build -o cmd/client/client ./cmd/client
CLIENT_SIZE=$(du -h cmd/client/client | cut -f1)
echo "✅ Client built: client/cmd/client/client ($CLIENT_SIZE)"
echo ""

# Check for config file
cd "$PROJECT_ROOT/server"
if [ ! -f config.yaml ]; then
    echo "⚠️  No config.yaml found. Creating from example..."
    if [ -f config.example.yaml ]; then
        cp config.example.yaml config.yaml
        echo "✅ Created config.yaml from example"
        echo ""
        echo "📝 Edit server/config.yaml to set your model path:"
        echo "   transcription:"
        echo "     model_path: \"/Users/$(whoami)/.cache/whisper/ggml-large-v3-turbo.bin\""
        echo ""
    fi
fi

echo "✅ Build complete!"
echo ""
echo "To run:"
echo "  1. Start server: ./server/cmd/server/server"
echo "  2. Start client: ./client/cmd/client/client"
echo "  3. Test recording: curl -X POST http://localhost:8081/start"
echo ""
echo "💡 With Metal GPU acceleration, expect ~40x realtime transcription speed!"
