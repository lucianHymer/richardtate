#!/bin/bash
set -e

echo "Installing RNNoise library..."

# Detect project root from script location
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
PROJECT_ROOT="$( cd "$SCRIPT_DIR/.." && pwd )"

# Install directory
INSTALL_DIR="$PROJECT_ROOT/deps/rnnoise"
mkdir -p "$INSTALL_DIR"

# Check if already installed
if [ -f "$INSTALL_DIR/lib/librnnoise.so" ]; then
    echo "✓ RNNoise already installed"
    exit 0
fi

# Clone xiph/rnnoise (official implementation)
if [ ! -d "$INSTALL_DIR/src" ]; then
    echo "Cloning xiph/rnnoise..."
    git clone https://github.com/xiph/rnnoise.git "$INSTALL_DIR/src"
fi

cd "$INSTALL_DIR/src"

# Detect number of CPU cores (cross-platform)
if command -v nproc &> /dev/null; then
    NCPU=$(nproc)
else
    NCPU=$(sysctl -n hw.ncpu)
fi

# Build
echo "Building RNNoise..."
./autogen.sh
./configure --prefix="$INSTALL_DIR"
make -j$NCPU
make install

echo "✓ RNNoise installed to $INSTALL_DIR"
echo ""
echo "To use:"
echo "export PKG_CONFIG_PATH=\"$INSTALL_DIR/lib/pkgconfig:\$PKG_CONFIG_PATH\""
echo "go build -tags rnnoise ..."
