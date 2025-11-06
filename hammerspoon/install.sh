#!/bin/bash
# Hammerspoon Installation Script for Streaming Transcription

set -e

echo "🎤 Installing Hammerspoon Integration for Streaming Transcription"
echo ""

# Check if Hammerspoon is installed
if ! command -v hs &> /dev/null; then
    echo "❌ Hammerspoon not found!"
    echo ""
    echo "Please install Hammerspoon first:"
    echo "  brew install --cask hammerspoon"
    echo ""
    echo "Or download from: https://www.hammerspoon.org/"
    exit 1
fi

echo "✅ Hammerspoon found"
echo ""

# Create Hammerspoon config directory
HAMMERSPOON_DIR="$HOME/.hammerspoon"
mkdir -p "$HAMMERSPOON_DIR"
echo "✅ Created config directory: $HAMMERSPOON_DIR"

# Get the script directory
SCRIPT_DIR="$( cd "$( dirname "${BASH_SOURCE[0]}" )" && pwd )"
INIT_LUA="$SCRIPT_DIR/init.lua"

# Copy or symlink the init.lua
if [ -f "$HAMMERSPOON_DIR/init.lua" ]; then
    echo ""
    echo "⚠️  Existing init.lua found at $HAMMERSPOON_DIR/init.lua"
    echo ""
    echo "Options:"
    echo "  1) Backup existing and install new init.lua"
    echo "  2) Append to existing init.lua"
    echo "  3) Cancel installation"
    echo ""
    read -p "Choose [1/2/3]: " choice

    case $choice in
        1)
            BACKUP="$HAMMERSPOON_DIR/init.lua.backup.$(date +%Y%m%d_%H%M%S)"
            mv "$HAMMERSPOON_DIR/init.lua" "$BACKUP"
            echo "✅ Backed up to: $BACKUP"
            cp "$INIT_LUA" "$HAMMERSPOON_DIR/init.lua"
            echo "✅ Installed new init.lua"
            ;;
        2)
            echo "" >> "$HAMMERSPOON_DIR/init.lua"
            echo "-- Streaming Transcription Integration (added $(date))" >> "$HAMMERSPOON_DIR/init.lua"
            cat "$INIT_LUA" >> "$HAMMERSPOON_DIR/init.lua"
            echo "✅ Appended to existing init.lua"
            ;;
        3)
            echo "❌ Installation cancelled"
            exit 0
            ;;
        *)
            echo "❌ Invalid choice"
            exit 1
            ;;
    esac
else
    cp "$INIT_LUA" "$HAMMERSPOON_DIR/init.lua"
    echo "✅ Installed init.lua"
fi

echo ""
echo "✅ Installation complete!"
echo ""
echo "Next steps:"
echo "  1. Grant Hammerspoon accessibility permissions:"
echo "     System Preferences → Security & Privacy → Privacy → Accessibility"
echo "     Add Hammerspoon and enable it"
echo ""
echo "  2. Reload Hammerspoon config:"
echo "     Click Hammerspoon menu → Reload Config"
echo "     (or press Cmd+Alt+Ctrl+R)"
echo ""
echo "  3. Start the client daemon:"
echo "     cd client && ./client"
echo ""
echo "  4. Press Ctrl+N to start recording!"
echo ""
echo "📖 See hammerspoon/README.md for detailed usage instructions."
