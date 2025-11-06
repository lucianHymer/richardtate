#!/bin/bash
#
# macOS Build Script for Streaming Transcription
#
# Prerequisites:
#   brew install whisper-cpp
#   brew install go
#
# Optional (for RNNoise noise suppression):
#   ./scripts/install-rnnoise-lib.sh
#   NOTE: Do NOT use 'brew install rnnoise' - it installs a different package
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

# Check for locally-built rnnoise (required on macOS)
# Note: Homebrew's rnnoise package does NOT work - you MUST build from source
LOCAL_RNNOISE="$PROJECT_ROOT/deps/rnnoise"
if [ -d "$LOCAL_RNNOISE/lib" ] && [ -f "$LOCAL_RNNOISE/lib/librnnoise.so" -o -f "$LOCAL_RNNOISE/lib/librnnoise.dylib" ]; then
    echo "✅ Found locally-built rnnoise at $LOCAL_RNNOISE"
    ENABLE_RNNOISE=true
    RNNOISE_PREFIX="$LOCAL_RNNOISE"
else
    echo "⚠️  rnnoise not installed"
    echo ""
    echo "RNNoise provides neural noise suppression (recommended for noisy environments)."
    echo "Note: Do NOT use 'brew install rnnoise' - it installs the wrong package."
    echo ""
    read -p "Would you like to install RNNoise now? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "🔨 Installing RNNoise from source..."
        "$SCRIPT_DIR/install-rnnoise-lib.sh"
        if [ -d "$LOCAL_RNNOISE/lib" ]; then
            echo "✅ RNNoise installed successfully!"
            ENABLE_RNNOISE=true
            RNNOISE_PREFIX="$LOCAL_RNNOISE"
        else
            echo "❌ RNNoise installation failed. Building without noise suppression."
        fi
        echo ""
    else
        echo "Continuing without RNNoise (no noise suppression)."
        echo ""
    fi
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
    export PKG_CONFIG_PATH="${RNNOISE_PREFIX}/lib/pkgconfig:${PKG_CONFIG_PATH}"
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

# Always create config directory and copy configs if they don't exist
CONFIG_DIR="$HOME/.config/richardtate"
mkdir -p "$CONFIG_DIR"

if [ ! -f "$CONFIG_DIR/server.yaml" ]; then
    cp "$PROJECT_ROOT/server/config.example.yaml" "$CONFIG_DIR/server.yaml"
    echo "✅ Created server config at $CONFIG_DIR/server.yaml"
fi
if [ ! -f "$CONFIG_DIR/client.yaml" ]; then
    cp "$PROJECT_ROOT/client/config.example.yaml" "$CONFIG_DIR/client.yaml"
    echo "✅ Created client config at $CONFIG_DIR/client.yaml"
fi
echo ""

# Ask about daemon setup
echo "Would you like to set up background daemon services?"
echo "This will:"
echo "  - Install launchd services for server and client"
echo "  - Auto-start services on login"
echo "  - Auto-restart on crash"
echo "  - Install 'richardtate' command for service control"
echo ""
read -p "Set up daemon services? (Y/n) " -n 1 -r
echo
if [[ ! $REPLY =~ ^[Nn]$ ]]; then
    echo "🔨 Setting up daemon services..."

    # Create logs directory
    LOGS_DIR="$CONFIG_DIR/logs"
    mkdir -p "$LOGS_DIR"

    # Install launchd plists
    PLIST_DIR="$HOME/Library/LaunchAgents"
    mkdir -p "$PLIST_DIR"

    sed "s|PROJECT_ROOT|$PROJECT_ROOT|g; s|HOME|$HOME|g" \
        "$SCRIPT_DIR/com.richardtate.server.plist" > "$PLIST_DIR/com.richardtate.server.plist"
    echo "✅ Server service installed"

    sed "s|PROJECT_ROOT|$PROJECT_ROOT|g; s|HOME|$HOME|g" \
        "$SCRIPT_DIR/com.richardtate.client.plist" > "$PLIST_DIR/com.richardtate.client.plist"
    echo "✅ Client service installed"

    # Install control script
    sudo cp "$SCRIPT_DIR/richardtate" /usr/local/bin/richardtate
    sudo chmod +x /usr/local/bin/richardtate
    echo "✅ Control script installed"

    echo ""
    echo "🎉 Daemon services configured!"
    echo ""

    # Check if services are already running
    if launchctl list | grep -q "com.richardtate.server" || launchctl list | grep -q "com.richardtate.client"; then
        echo "⚠️  Services are currently running with old binaries."
        echo ""
        read -p "Restart services to use new binaries? (Y/n) " -n 1 -r
        echo
        if [[ ! $REPLY =~ ^[Nn]$ ]]; then
            launchctl unload "$PLIST_DIR/com.richardtate.server.plist" 2>/dev/null || true
            launchctl unload "$PLIST_DIR/com.richardtate.client.plist" 2>/dev/null || true
            sleep 1
            launchctl load "$PLIST_DIR/com.richardtate.server.plist" 2>/dev/null || true
            launchctl load "$PLIST_DIR/com.richardtate.client.plist" 2>/dev/null || true
            echo "✅ Services restarted with new binaries"
        else
            echo "Remember to restart: richardtate restart"
        fi
        echo ""
    else
        echo "Next steps:"
        echo "  1. Calibrate VAD: cd $PROJECT_ROOT/client && ./client --calibrate"
        echo "  2. Start services: richardtate start"
        echo "  3. Check status:   richardtate status"
        echo "  4. View logs:      richardtate logs"
        echo ""
    fi
else
    echo ""
    echo "To run manually:"
    echo "  1. Start server: ./server/cmd/server/server"
    echo "  2. Start client: ./client/cmd/client/client"
    echo "  3. Test recording: curl -X POST http://localhost:8081/start"
    echo ""
fi

echo "💡 With Metal GPU acceleration, expect ~40x realtime transcription speed!"
