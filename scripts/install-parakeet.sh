#!/bin/bash
# Install Python dependencies for Parakeet MLX worker

set -e

echo "📦 Installing Python dependencies for Parakeet MLX..."
echo ""

# Check if pip3 is available
if ! command -v pip3 &> /dev/null; then
    echo "❌ pip3 not found. Please install Python 3 first."
    exit 1
fi

# Install requirements
pip3 install -r "$(dirname "$0")/requirements-parakeet.txt"

echo ""
echo "✅ Parakeet MLX dependencies installed successfully!"
echo ""
echo "You can now use engine: \"parakeet\" in your server config."
