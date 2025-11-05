#!/bin/bash
set -e

echo "🚀 Setting up Fedora Development Environment for Streaming Transcription"
echo "========================================================================="
echo ""

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# Check if we're on Fedora
if ! grep -q "Fedora" /etc/os-release; then
    echo "⚠️  Warning: This script is designed for Fedora. Proceed with caution."
    read -p "Continue anyway? (y/N) " -n 1 -r
    echo
    if [[ ! $REPLY =~ ^[Yy]$ ]]; then
        exit 1
    fi
fi

echo "📦 Installing system dependencies..."
echo ""

# Build essentials for whisper.cpp
echo "Installing build tools (cmake, make, gcc-c++)..."
sudo dnf install -y cmake make gcc-c++

# Audio libraries for malgo
echo "Installing audio libraries (ALSA, PulseAudio)..."
sudo dnf install -y alsa-lib-devel pulseaudio-libs-devel

# Optional but useful development tools
echo "Installing additional development tools..."
sudo dnf install -y pkg-config

echo ""
echo "✅ System dependencies installed!"
echo ""

# Now run the project-specific installation scripts
echo "📥 Running project installation scripts..."
echo ""

# Install whisper.cpp
if [ -f "$SCRIPT_DIR/scripts/install-whisper.sh" ]; then
    echo "1️⃣  Installing Whisper.cpp..."
    bash "$SCRIPT_DIR/scripts/install-whisper.sh"
    echo ""
else
    echo "❌ scripts/install-whisper.sh not found!"
    exit 1
fi

# Download Whisper models
if [ -f "$SCRIPT_DIR/scripts/download-models.sh" ]; then
    echo "2️⃣  Downloading Whisper models..."
    bash "$SCRIPT_DIR/scripts/download-models.sh"
    echo ""
else
    echo "❌ scripts/download-models.sh not found!"
    exit 1
fi

# Download RNNoise model
if [ -f "$SCRIPT_DIR/scripts/download-rnnoise.sh" ]; then
    echo "3️⃣  Downloading RNNoise model..."
    bash "$SCRIPT_DIR/scripts/download-rnnoise.sh"
    echo ""
else
    echo "❌ scripts/download-rnnoise.sh not found!"
    exit 1
fi

echo ""
echo "🎉 Installation complete!"
echo ""
echo "📋 Next steps:"
echo "1. Set up CGO environment (run this in your current shell):"
echo "   source ./scripts/setup-env.sh"
echo ""
echo "2. Build the project:"
echo "   make build"
echo ""
echo "3. Run the server:"
echo "   ./server/cmd/server/server"
echo ""
echo "4. In another terminal, run the client:"
echo "   ./client/cmd/client/client"
echo ""
echo "💡 Remember: You must source setup-env.sh in EVERY new shell session!"
echo ""
