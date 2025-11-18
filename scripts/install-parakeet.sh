#!/bin/bash
# Install Python dependencies for Parakeet MLX worker

set -e

# Use provided Python path or default to python3
PYTHON="${1:-python3}"

echo "📦 Installing Python dependencies for Parakeet MLX..."
echo "Using Python: $PYTHON"
echo ""

# Check if Python is available
if ! command -v "$PYTHON" &> /dev/null; then
    echo "❌ $PYTHON not found. Please install Python 3 first."
    exit 1
fi

# Get the pip module for this Python
echo "Installing with: $PYTHON -m pip"
"$PYTHON" -m pip install -r "$(dirname "$0")/requirements-parakeet.txt"

echo ""
echo "✅ Parakeet MLX dependencies installed successfully!"
echo ""

# Get absolute path to parakeet_worker_streaming.py
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
WORKER_SCRIPT="$SCRIPT_DIR/parakeet_worker_streaming.py"

echo "Add this to your server config:"
echo ""
echo "transcription:"
echo "  engine: \"parakeet\""
echo "  parakeet:"
echo "    model_id: \"mlx-community/parakeet-tdt-0.6b-v3\""
echo "    script_path: \"$WORKER_SCRIPT\""
echo "    python_path: \"$PYTHON\""
echo ""
