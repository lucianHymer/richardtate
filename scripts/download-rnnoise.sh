#!/bin/bash
set -e

# RNNoise Model Download Script
# Downloads pre-trained RNNoise model for noise suppression

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODELS_DIR="$PROJECT_ROOT/models/rnnoise"

# RNNoise model from GregorR's trained models
# This is the "leavened-quisling" model - good general-purpose noise suppression
MODEL_URL="https://github.com/GregorR/rnnoise-models/raw/refs/heads/master/leavened-quisling-2018-08-31/lq.rnnn"
MODEL_NAME="lq.rnnn"

echo "🔇 Downloading RNNoise model..."
echo "Destination: $MODELS_DIR"
mkdir -p "$MODELS_DIR"

MODEL_PATH="$MODELS_DIR/$MODEL_NAME"

if [ -f "$MODEL_PATH" ]; then
    echo "✓ $MODEL_NAME already downloaded ($(du -h "$MODEL_PATH" | cut -f1))"
    exit 0
fi

echo "📥 Downloading $MODEL_NAME from GregorR/rnnoise-models..."

# Try curl first, fall back to wget
if command -v curl &> /dev/null; then
    curl -L --progress-bar -o "$MODEL_PATH" "$MODEL_URL"
elif command -v wget &> /dev/null; then
    wget --show-progress -O "$MODEL_PATH" "$MODEL_URL"
else
    echo "❌ Neither curl nor wget found. Please install one."
    exit 1
fi

echo "✓ Downloaded $MODEL_NAME ($(du -h "$MODEL_PATH" | cut -f1))"
echo ""
echo "✅ RNNoise model ready!"
echo ""
echo "💡 Model location: $MODEL_PATH"
echo "💡 This model will be used for real-time noise suppression in Phase 2"
