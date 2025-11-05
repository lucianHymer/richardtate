#!/bin/bash
set -e

# Whisper Model Download Script
# Downloads GGML models from Hugging Face

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PROJECT_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
MODELS_DIR="$PROJECT_ROOT/models"

BASE_URL="https://huggingface.co/ggerganov/whisper.cpp/resolve/main"

# Available models (uncomment what you need)
# Tiny models (~75MB) - Fast, less accurate
# MODELS=("tiny" "tiny.en")

# Base models (~142MB) - Good balance
# MODELS=("base" "base.en")

# Small models (~466MB) - Better accuracy
# MODELS=("small" "small.en")

# Medium models (~1.5GB) - High accuracy
# MODELS=("medium" "medium.en")

# Large models (~3GB) - Best accuracy
# MODELS=("large-v1" "large-v2" "large-v3")

# Turbo model (~1.6GB) - Fast + accurate (RECOMMENDED)
MODELS=("large-v3-turbo")

echo "🎤 Downloading Whisper models..."
echo "Destination: $MODELS_DIR"
mkdir -p "$MODELS_DIR"

for model in "${MODELS[@]}"; do
    MODEL_FILE="ggml-${model}.bin"
    MODEL_PATH="$MODELS_DIR/$MODEL_FILE"

    if [ -f "$MODEL_PATH" ]; then
        echo "✓ $MODEL_FILE already downloaded ($(du -h "$MODEL_PATH" | cut -f1))"
        continue
    fi

    echo "📥 Downloading $MODEL_FILE..."
    DOWNLOAD_URL="$BASE_URL/$MODEL_FILE"

    # Try curl first, fall back to wget
    if command -v curl &> /dev/null; then
        curl -L --progress-bar -o "$MODEL_PATH" "$DOWNLOAD_URL"
    elif command -v wget &> /dev/null; then
        wget --show-progress -O "$MODEL_PATH" "$DOWNLOAD_URL"
    else
        echo "❌ Neither curl nor wget found. Please install one."
        exit 1
    fi

    echo "✓ Downloaded $MODEL_FILE ($(du -h "$MODEL_PATH" | cut -f1))"
done

echo ""
echo "✅ Model download complete!"
echo ""
echo "📦 Downloaded models:"
ls -lh "$MODELS_DIR"
echo ""
echo "💡 To use a different model, edit this script and change the MODELS array"
