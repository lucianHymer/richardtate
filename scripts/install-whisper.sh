#!/bin/bash
set -e

# Whisper.cpp Installation Script
# This script downloads and builds whisper.cpp with Go bindings

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
DEPS_DIR="$PROJECT_ROOT/deps"
WHISPER_DIR="$DEPS_DIR/whisper.cpp"
MODELS_DIR="$PROJECT_ROOT/models"

echo "🎤 Installing Whisper.cpp..."
echo "Project root: $PROJECT_ROOT"

# Create directories
mkdir -p "$DEPS_DIR"
mkdir -p "$MODELS_DIR"

# Clone whisper.cpp if not already present
if [ ! -d "$WHISPER_DIR" ]; then
    echo "📥 Cloning whisper.cpp..."
    cd "$DEPS_DIR"
    git clone https://github.com/ggml-org/whisper.cpp.git
    cd "$WHISPER_DIR"
else
    echo "✓ whisper.cpp already cloned"
    cd "$WHISPER_DIR"
    echo "🔄 Pulling latest changes..."
    git pull
fi

# Build whisper.cpp static library
echo "🔨 Building libwhisper.a..."
mkdir -p build
cd build
cmake .. -DCMAKE_BUILD_TYPE=Release -DBUILD_SHARED_LIBS=OFF
cmake --build . -j $(nproc) --config Release

# Verify library was built
if [ ! -f "$WHISPER_DIR/build/libwhisper.a" ]; then
    echo "❌ Failed to build libwhisper.a"
    exit 1
fi
echo "✓ libwhisper.a built successfully"

# Create symlink for easy access
ln -sf "$WHISPER_DIR/build/libwhisper.a" "$DEPS_DIR/libwhisper.a"

echo ""
echo "✅ Whisper.cpp installation complete!"
echo ""
echo "📦 Next steps:"
echo "1. Download models with: ./scripts/download-models.sh"
echo "2. Set environment variables before building server:"
echo "   export CGO_CFLAGS=\"-I$WHISPER_DIR/include -I$WHISPER_DIR/ggml/include\""
echo "   export CGO_LDFLAGS=\"-L$WHISPER_DIR/build -lwhisper\""
echo "   export LIBRARY_PATH=\"$WHISPER_DIR/build:\$LIBRARY_PATH\""
echo ""
