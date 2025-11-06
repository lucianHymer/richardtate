#!/bin/bash
set -e

echo "Installing RNNoise library..."

# Install directory
INSTALL_DIR="/workspace/project/deps/rnnoise"
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

# Build
echo "Building RNNoise..."
./autogen.sh
./configure --prefix="$INSTALL_DIR"
make -j$(nproc)
make install

echo "✓ RNNoise installed to $INSTALL_DIR"
echo ""
echo "To use:"
echo "export PKG_CONFIG_PATH=\"$INSTALL_DIR/lib/pkgconfig:\$PKG_CONFIG_PATH\""
echo "go build -tags rnnoise ..."
