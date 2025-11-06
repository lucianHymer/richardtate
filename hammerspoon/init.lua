-- Streaming Transcription Hammerspoon Integration
-- Simple hotkey-triggered voice transcription with direct text insertion

-- Load required extensions (use hs. prefix for safety)
-- Some modules work with require(), others need hs. global access

-- Configuration
local config = {
    daemonURL = "http://localhost:8081",
    wsURL = "ws://localhost:8081/transcriptions",
    hotkey = {mods = {"ctrl"}, key = "n"},
}

-- State
local state = {
    recording = false,
    ws = nil,
    indicator = nil,
}

-- HTTP helper
local function httpRequest(method, path, callback)
    local url = config.daemonURL .. path
    hs.http.doAsyncRequest(url, method, nil, nil, function(status, body, headers)
        if callback then
            callback(status, body)
        end
    end)
end

-- Indicator UI (minimal floating window)
local function createIndicator()
    local mainScreen = hs.screen.mainScreen()
    local frame = mainScreen:frame()

    -- Position: top-right corner with 20px margin
    local width = 200
    local height = 40
    local x = frame.x + frame.w - width - 20
    local y = frame.y + 20

    local canvasObj = hs.canvas.new({x = x, y = y, w = width, h = height})

    -- Background (semi-transparent dark)
    canvasObj:appendElements({
        type = "rectangle",
        action = "fill",
        fillColor = {red = 0.1, green = 0.1, blue = 0.1, alpha = 0.9},
        roundedRectRadii = {xRadius = 8, yRadius = 8},
    })

    -- Red recording dot
    canvasObj:appendElements({
        type = "circle",
        action = "fill",
        fillColor = {red = 1.0, green = 0.0, blue = 0.0, alpha = 1.0},
        center = {x = 20, y = 20},
        radius = 6,
    })

    -- Text: "Recording..."
    canvasObj:appendElements({
        type = "text",
        text = "Recording...",
        textColor = {red = 1.0, green = 1.0, blue = 1.0, alpha = 1.0},
        textSize = 16,
        textAlignment = "left",
        frame = {x = 35, y = 10, w = 150, h = 20},
    })

    return canvasObj
end

local function showIndicator()
    if state.indicator then
        state.indicator:delete()
    end
    state.indicator = createIndicator()
    state.indicator:show()
end

local function hideIndicator()
    if state.indicator then
        state.indicator:delete()
        state.indicator = nil
    end
end

-- WebSocket connection for receiving transcriptions
local function connectWebSocket()
    if state.ws then
        state.ws:close()
    end

    state.ws = hs.websocket.new(config.wsURL, function(event, message)
        print("🔔 WebSocket event: " .. tostring(event) .. " | message: " .. tostring(message))

        if event == "message" then
            local success, data = pcall(hs.json.decode, message)
            if success and data.chunk then
                print("📝 Received chunk: " .. data.chunk)
                -- Insert text directly at cursor position!
                hs.eventtap.keyStrokes(data.chunk)
                print("✓ Typed chunk")
            elseif success and data.final then
                -- Recording complete (optional: could show notification)
                print("Transcription complete")
            else
                print("⚠️ Failed to decode or no chunk: " .. tostring(message))
            end
        elseif event == "open" then
            print("WebSocket connected")
        elseif event == "closed" then
            print("WebSocket closed")
        elseif event == "fail" then
            print("WebSocket failed: " .. tostring(message))
        end
    end)

    if not state.ws then
        print("ERROR: Failed to create WebSocket object")
        return
    end

    -- WebSocket automatically connects, no need to call :connect()
end

local function disconnectWebSocket()
    if state.ws then
        state.ws:close()
        state.ws = nil
    end
end

-- Start recording
local function startRecording()
    print("Starting recording...")

    -- Show indicator
    showIndicator()

    -- Connect WebSocket for transcriptions
    connectWebSocket()

    -- Start daemon recording
    httpRequest("POST", "/start", function(status, body)
        if status == 200 then
            print("Recording started")
            state.recording = true
        else
            print("Failed to start recording: " .. tostring(status))
            hideIndicator()
            disconnectWebSocket()
        end
    end)
end

-- Stop recording
local function stopRecording()
    print("Stopping recording...")

    -- Hide indicator
    hideIndicator()

    -- Stop daemon recording
    httpRequest("POST", "/stop", function(status, body)
        if status == 200 then
            print("Recording stopped")
        else
            print("Failed to stop recording: " .. tostring(status))
        end
    end)

    -- Disconnect WebSocket (give it a moment for final chunks)
    hs.timer.doAfter(1.0, function()
        disconnectWebSocket()
        state.recording = false
    end)
end

-- Toggle recording on hotkey
local function toggleRecording()
    if state.recording then
        stopRecording()
    else
        startRecording()
    end
end

-- Bind hotkey
hs.hotkey.bind(config.hotkey.mods, config.hotkey.key, toggleRecording)

-- Cleanup on reload
hs.hotkey.bind({"cmd", "alt", "ctrl"}, "r", function()
    if state.recording then
        stopRecording()
    end
    hs.reload()
end)

-- Notification on load
hs.notify.new({
    title = "Streaming Transcription",
    informativeText = "Press Ctrl+N to start/stop recording"
}):send()

print("Streaming Transcription loaded. Press Ctrl+N to toggle recording.")
