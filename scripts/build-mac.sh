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

# Check for FFmpeg (required for Parakeet MLX)
FFMPEG_INSTALLED=false
if command -v ffmpeg &> /dev/null; then
    echo "✅ FFmpeg is installed"
    FFMPEG_INSTALLED=true
else
    echo "⚠️  FFmpeg not installed"
    echo ""
    echo "FFmpeg is required for Parakeet MLX to load audio files."
    echo ""
    read -p "Would you like to install FFmpeg now? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        echo "🔨 Installing FFmpeg via Homebrew..."
        if brew install ffmpeg; then
            echo "✅ FFmpeg installed successfully!"
            FFMPEG_INSTALLED=true
        else
            echo "❌ FFmpeg installation failed."
        fi
        echo ""
    else
        echo "Continuing without FFmpeg. Note: Parakeet MLX will not work without it."
        echo ""
    fi
fi

# Check for Parakeet MLX (optional - for alternative ASR engine)
PARAKEET_INSTALLED=false
PARAKEET_MODEL_PATH="$PROJECT_ROOT/models/parakeet/parakeet-tdt-0.6b"

echo "🦜 Checking Parakeet MLX installation..."
if python3 -c "import parakeet_mlx" 2>/dev/null; then
    echo "✅ Parakeet MLX is installed"
    PARAKEET_INSTALLED=true

    # Check if model exists
    if [ -d "$PARAKEET_MODEL_PATH" ]; then
        echo "✅ Parakeet model found at: $PARAKEET_MODEL_PATH"
    else
        echo "⚠️  Parakeet model not found"
        echo ""
        echo "Parakeet models enable an alternative ASR engine optimized for Apple Silicon."
        echo ""
        read -p "Would you like to download the Parakeet model now? (~600MB) (y/N) " -n 1 -r
        echo
        if [[ $REPLY =~ ^[Yy]$ ]]; then
            echo "📥 Downloading Parakeet model..."
            mkdir -p "$PROJECT_ROOT/models/parakeet"

            # Create a temporary Python script to download the model
            cat > /tmp/download_parakeet.py << 'EOF'
import sys
from parakeet_mlx import from_pretrained
import shutil
import os

model_id = "mlx-community/parakeet-tdt-0.6b-v3"
target_dir = sys.argv[1]

print(f"Downloading {model_id}...")
try:
    # This downloads to cache first
    model = from_pretrained(model_id)
    print(f"Model downloaded successfully")

    # The model is cached in ~/.cache/parakeet-mlx/
    # We'll keep it there and just note the location
    print(f"Model is cached and ready to use with ID: {model_id}")

    # Create a marker file to indicate the model is ready
    marker_path = os.path.join(target_dir, "parakeet-tdt-0.6b")
    os.makedirs(marker_path, exist_ok=True)
    with open(os.path.join(marker_path, "MODEL_ID"), "w") as f:
        f.write(model_id)

except Exception as e:
    print(f"Error downloading model: {e}")
    sys.exit(1)
EOF

            if python3 /tmp/download_parakeet.py "$PROJECT_ROOT/models/parakeet"; then
                echo "✅ Parakeet model downloaded successfully!"
            else
                echo "❌ Failed to download Parakeet model. You can try again later."
            fi
            rm /tmp/download_parakeet.py
            echo ""
        else
            echo "Skipping Parakeet model download. You can download it later if needed."
            echo ""
        fi
    fi
else
    echo "⚠️  Parakeet MLX not installed"
    echo ""
    echo "Parakeet MLX provides an alternative ASR engine optimized for Apple Silicon."
    echo "It offers better performance than Whisper on Mac with features like word-level timestamps."
    echo ""

    # Warn if FFmpeg is not installed
    if [ "$FFMPEG_INSTALLED" = false ]; then
        echo "⚠️  WARNING: FFmpeg is not installed. Parakeet MLX requires FFmpeg to work."
        echo "   Install FFmpeg first: brew install ffmpeg"
        echo ""
    fi

    read -p "Would you like to install Parakeet MLX now? (y/N) " -n 1 -r
    echo
    if [[ $REPLY =~ ^[Yy]$ ]]; then
        if "$PROJECT_ROOT/scripts/install-parakeet.sh"; then
            PARAKEET_INSTALLED=true

            # Now offer to download the model
            echo ""
            read -p "Download Parakeet model now? (~600MB) (y/N) " -n 1 -r
            echo
            if [[ $REPLY =~ ^[Yy]$ ]]; then
                echo "📥 Downloading Parakeet model..."
                mkdir -p "$PROJECT_ROOT/models/parakeet"

                # Same download script as above
                cat > /tmp/download_parakeet.py << 'EOF'
import sys
from parakeet_mlx import from_pretrained
import shutil
import os

model_id = "mlx-community/parakeet-tdt-0.6b-v3"
target_dir = sys.argv[1]

print(f"Downloading {model_id}...")
try:
    # This downloads to cache first
    model = from_pretrained(model_id)
    print(f"Model downloaded successfully")

    # The model is cached in ~/.cache/parakeet-mlx/
    # We'll keep it there and just note the location
    print(f"Model is cached and ready to use with ID: {model_id}")

    # Create a marker file to indicate the model is ready
    marker_path = os.path.join(target_dir, "parakeet-tdt-0.6b")
    os.makedirs(marker_path, exist_ok=True)
    with open(os.path.join(marker_path, "MODEL_ID"), "w") as f:
        f.write(model_id)

except Exception as e:
    print(f"Error downloading model: {e}")
    sys.exit(1)
EOF

                if python3 /tmp/download_parakeet.py "$PROJECT_ROOT/models/parakeet"; then
                    echo "✅ Parakeet model ready!"
                else
                    echo "❌ Failed to download model. You can try again later."
                fi
                rm /tmp/download_parakeet.py
            fi
        else
            echo "❌ Failed to install Parakeet MLX. Continuing without it."
        fi
        echo ""
    else
        echo "Continuing without Parakeet MLX (using Whisper only)."
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

export SDKROOT="/Library/Developer/CommandLineTools/SDKs/MacOSX.sdk"

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

cd "$PROJECT_ROOT"
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

# Summary of available ASR engines
echo ""
echo "📊 Available ASR Engines:"
echo "  ✅ Whisper - Traditional, robust transcription"
if [ "$PARAKEET_INSTALLED" = true ] && [ -d "$PARAKEET_MODEL_PATH" ]; then
    if [ "$FFMPEG_INSTALLED" = true ]; then
        echo "  ✅ Parakeet MLX - Apple Silicon optimized, word-level timestamps"
    else
        echo "  ⚠️  Parakeet MLX - Installed but FFmpeg missing (required for audio loading)"
        echo "     Install FFmpeg: brew install ffmpeg"
    fi
    echo ""
    echo "  To switch between engines, edit ~/.config/richardtate/server.yaml:"
    echo "    transcription:"
    echo "      engine: \"whisper\"  # or \"parakeet\""
elif [ "$PARAKEET_INSTALLED" = true ]; then
    echo "  ⚠️  Parakeet MLX - Installed but model not downloaded"
    echo ""
    echo "  To download model: python3 -c 'from parakeet_mlx import from_pretrained; from_pretrained(\"mlx-community/parakeet-tdt-0.6b-v3\")'"
else
    echo "  ⚠️  Parakeet MLX - Not installed (run build script again to install)"
fi
