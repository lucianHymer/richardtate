-- Example Configuration for Streaming Transcription Hammerspoon Integration
-- Copy this to the top of your ~/.hammerspoon/init.lua and customize as needed

local config = {
    -- Client daemon URL (default: http://localhost:8081)
    daemonURL = "http://localhost:8081",

    -- WebSocket URL for transcriptions (default: ws://localhost:8081/transcriptions)
    wsURL = "ws://localhost:8081/transcriptions",

    -- Hotkey configuration
    hotkey = {
        mods = {"ctrl"},  -- Modifier keys: "cmd", "alt", "ctrl", "shift"
        key = "n",        -- Key to press
    },

    -- Indicator positioning
    indicator = {
        position = "top-right",  -- Position on screen
        xOffset = 20,            -- Pixels from edge
        yOffset = 20,            -- Pixels from edge
        width = 200,             -- Indicator width
        height = 40,             -- Indicator height
    },

    -- Visual customization
    visual = {
        backgroundColor = {red = 0.1, green = 0.1, blue = 0.1, alpha = 0.9},
        recordingColor = {red = 1.0, green = 0.0, blue = 0.0, alpha = 1.0},
        textColor = {red = 1.0, green = 1.0, blue = 1.0, alpha = 1.0},
        fontSize = 16,
    },

    -- Behavior options
    behavior = {
        showNotificationOnLoad = true,   -- Show notification when script loads
        showNotificationOnStart = false, -- Show notification when recording starts
        showNotificationOnStop = false,  -- Show notification when recording stops
        autoReconnectWebSocket = true,   -- Automatically reconnect WebSocket on failure
    },

    -- Debug options
    debug = {
        enabled = false,                 -- Enable debug logging to Hammerspoon console
        logWebSocketMessages = false,    -- Log every WebSocket message
        logHTTPRequests = false,         -- Log HTTP requests to daemon
    },
}

-- Alternative hotkey examples:
-- Cmd+Shift+V: {mods = {"cmd", "shift"}, key = "v"}
-- Alt+R: {mods = {"alt"}, key = "r"}
-- F13: {mods = {}, key = "f13"}  -- Good for keyboards with function keys

-- Alternative indicator positions:
-- Top-left: position = "top-left"
-- Bottom-right: position = "bottom-right"
-- Bottom-left: position = "bottom-left"
-- Center: position = "center"
