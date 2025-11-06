#!/bin/bash
set -e

# Voice Notes - macOS Installation Script
# Installs all dependencies and sets up the environment

echo "================================"
echo "Voice Notes - macOS Installer"
echo "================================"
echo ""

# Color output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

info() {
    echo -e "${BLUE}ℹ${NC}  $1"
}

success() {
    echo -e "${GREEN}✓${NC}  $1"
}

warn() {
    echo -e "${YELLOW}⚠${NC}  $1"
}

error() {
    echo -e "${RED}✗${NC}  $1"
}

# Detect project root
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"
cd "$PROJECT_ROOT"

info "Project root: $PROJECT_ROOT"
echo ""

# Check for Homebrew
echo "Checking dependencies..."
if ! command -v brew &> /dev/null; then
    error "Homebrew not found"
    echo "  Install from: https://brew.sh"
    exit 1
fi
success "Homebrew installed"

# Check for Go
if ! command -v go &> /dev/null; then
    error "Go not found"
    echo "  Install with: brew install go"
    exit 1
fi
GO_VERSION=$(go version | awk '{print $3}')
success "Go installed ($GO_VERSION)"

# Install Whisper.cpp
echo ""
echo "Installing Whisper.cpp..."
if [ -f "$PROJECT_ROOT/deps/whisper.cpp/build/src/libwhisper.a" ]; then
    success "Whisper.cpp already built"
else
    info "Building Whisper.cpp from source (this may take 5-10 minutes)..."

    # Check if whisper.cpp is cloned
    if [ ! -d "$PROJECT_ROOT/deps/whisper.cpp" ]; then
        info "Cloning whisper.cpp repository..."
        mkdir -p "$PROJECT_ROOT/deps"
        git clone https://github.com/ggml-org/whisper.cpp.git "$PROJECT_ROOT/deps/whisper.cpp"
    fi

    cd "$PROJECT_ROOT/deps/whisper.cpp"

    # Build with Metal acceleration (macOS GPU)
    mkdir -p build
    cd build
    cmake .. -DGGML_METAL=ON
    make -j$(sysctl -n hw.ncpu)

    cd "$PROJECT_ROOT"
    success "Whisper.cpp built successfully"
fi

# Download Whisper model
echo ""
echo "Downloading Whisper model..."
WHISPER_MODEL_DIR="$PROJECT_ROOT/models"
WHISPER_MODEL="$WHISPER_MODEL_DIR/ggml-large-v3-turbo.bin"

if [ -f "$WHISPER_MODEL" ]; then
    MODEL_SIZE=$(du -h "$WHISPER_MODEL" | cut -f1)
    success "Whisper model already downloaded ($MODEL_SIZE)"
else
    info "Downloading ggml-large-v3-turbo.bin (~1.6GB)..."
    mkdir -p "$WHISPER_MODEL_DIR"

    curl -L -o "$WHISPER_MODEL" \
        "https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3-turbo.bin"

    success "Whisper model downloaded"
fi

# Install RNNoise
echo ""
echo "Installing RNNoise..."
if [ -f "$PROJECT_ROOT/deps/rnnoise/lib/librnnoise.a" ]; then
    success "RNNoise already built"
else
    info "Building RNNoise from source..."

    # Run the install script
    "$SCRIPT_DIR/install-rnnoise-lib.sh"

    success "RNNoise built successfully"
fi

# Download RNNoise model
echo ""
echo "Downloading RNNoise model..."
RNNOISE_MODEL_DIR="$PROJECT_ROOT/models/rnnoise"
RNNOISE_MODEL="$RNNOISE_MODEL_DIR/lq.rnnn"

if [ -f "$RNNOISE_MODEL" ]; then
    success "RNNoise model already downloaded"
else
    info "Downloading leavened-quisling model..."
    mkdir -p "$RNNOISE_MODEL_DIR"

    curl -L -o "$RNNOISE_MODEL" \
        "https://github.com/GregorR/rnnoise-models/raw/refs/heads/master/leavened-quisling-2018-08-31/lq.rnnn"

    success "RNNoise model downloaded"
fi

# Install Hammerspoon (optional)
echo ""
if command -v hs &> /dev/null; then
    success "Hammerspoon already installed"
else
    warn "Hammerspoon not installed (optional, for system-wide hotkey)"
    echo "  Install with: brew install --cask hammerspoon"
    echo "  Then run: cd $PROJECT_ROOT/hammerspoon && ./install.sh"
fi

# Create config directory
echo ""
echo "Setting up configuration..."
CONFIG_DIR="$HOME/.config/richardtate"
if [ ! -d "$CONFIG_DIR" ]; then
    mkdir -p "$CONFIG_DIR"
    success "Created config directory: $CONFIG_DIR"
else
    success "Config directory exists: $CONFIG_DIR"
fi

# Copy client config if needed
if [ ! -f "$CONFIG_DIR/client.yaml" ]; then
    info "Creating client config from example..."
    cp "$PROJECT_ROOT/client/config.example.yaml" "$CONFIG_DIR/client.yaml"
    success "Client config created at $CONFIG_DIR/client.yaml"
else
    warn "Client config already exists at $CONFIG_DIR/client.yaml"
    echo "  Review client/config.example.yaml for new options"
fi

# Copy server config if needed
if [ ! -f "$CONFIG_DIR/server.yaml" ]; then
    info "Creating server config from example..."
    cp "$PROJECT_ROOT/server/config.example.yaml" "$CONFIG_DIR/server.yaml"
    success "Server config created at $CONFIG_DIR/server.yaml"
else
    warn "Server config already exists at $CONFIG_DIR/server.yaml"
    echo "  Review server/config.example.yaml for new options"
fi

# Build client and server
echo ""
echo "Building client and server..."

# Build server with RNNoise
info "Building server (with RNNoise)..."
"$SCRIPT_DIR/build-mac.sh"
success "Server built"

# Build client
info "Building client..."
cd "$PROJECT_ROOT/client"
go build -o "$PROJECT_ROOT/client/client" ./cmd/client
cd "$PROJECT_ROOT"
success "Client built"

# Set up launchd services
echo ""
echo "Setting up launchd services..."

# Create logs directory
LOGS_DIR="$CONFIG_DIR/logs"
if [ ! -d "$LOGS_DIR" ]; then
    mkdir -p "$LOGS_DIR"
    success "Created logs directory: $LOGS_DIR"
else
    success "Logs directory exists: $LOGS_DIR"
fi

# Install launchd plists
PLIST_DIR="$HOME/Library/LaunchAgents"
mkdir -p "$PLIST_DIR"

# Process server plist (replace placeholders)
info "Installing server launchd service..."
sed "s|PROJECT_ROOT|$PROJECT_ROOT|g; s|HOME|$HOME|g" \
    "$SCRIPT_DIR/com.richardtate.server.plist" > "$PLIST_DIR/com.richardtate.server.plist"
success "Server service installed"

# Process client plist (replace placeholders)
info "Installing client launchd service..."
sed "s|PROJECT_ROOT|$PROJECT_ROOT|g; s|HOME|$HOME|g" \
    "$SCRIPT_DIR/com.richardtate.client.plist" > "$PLIST_DIR/com.richardtate.client.plist"
success "Client service installed"

# Install control script
info "Installing richardtate control script..."
sudo cp "$SCRIPT_DIR/richardtate" /usr/local/bin/richardtate
sudo chmod +x /usr/local/bin/richardtate
success "Control script installed to /usr/local/bin/richardtate"

# Installation complete
echo ""
echo "================================"
success "Installation complete!"
echo "================================"
echo ""
echo "Next steps:"
echo ""
echo "1. Calibrate VAD threshold (one-time setup):"
echo "   cd $PROJECT_ROOT/client"
echo "   ./client --calibrate"
echo ""
echo "2. Start the services:"
echo "   richardtate start"
echo ""
echo "3. Check service status:"
echo "   richardtate status"
echo ""
echo "4. View logs:"
echo "   richardtate logs"
echo ""
echo "5. (Optional) Install Hammerspoon integration:"
echo "   brew install --cask hammerspoon"
echo "   cd $PROJECT_ROOT/hammerspoon && ./install.sh"
echo "   Press Ctrl+N to start/stop recording"
echo ""
echo "Control Commands:"
echo "  richardtate start    - Start server and client"
echo "  richardtate stop     - Stop server and client"
echo "  richardtate restart  - Restart both services"
echo "  richardtate status   - Show service status"
echo "  richardtate logs     - Tail service logs"
echo ""
echo "Configuration: $CONFIG_DIR/"
echo "Logs: $LOGS_DIR/"
echo ""
