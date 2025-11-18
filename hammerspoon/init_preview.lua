-- Hammerspoon configuration for streaming transcription with preview window
-- This version shows a non-interactive preview that updates with each transcription

local recording = false
local ws = nil
local indicator = nil
local previewWindow = nil

-- Configuration
local CLIENT_URL = "http://localhost:8081"
local WS_URL = "ws://localhost:8081/transcriptions"

-- Create preview window (non-interactive, just displays text)
function createPreviewWindow()
    -- Position it nicely on screen
    local screen = hs.screen.mainScreen():frame()
    local windowWidth = 600
    local windowHeight = 400
    local windowX = screen.x + screen.w - windowWidth - 20  -- Top right with margin
    local windowY = screen.y + 50

    previewWindow = hs.webview.new({
        x = windowX,
        y = windowY,
        w = windowWidth,
        h = windowHeight
    })

    -- Make it a utility window (floats above other windows)
    previewWindow:windowStyle({"utility", "titled", "closable"})
    previewWindow:windowTitle("Transcription Preview (Live)")
    previewWindow:allowTextEntry(false)  -- No interaction

    -- Simple HTML with auto-scrolling
    previewWindow:html([[
        <!DOCTYPE html>
        <html>
        <head>
            <style>
                body {
                    font-family: -apple-system, system-ui, sans-serif;
                    padding: 20px;
                    background: #f5f5f5;
                    color: #333;
                    font-size: 16px;
                    line-height: 1.6;
                }
                #preview {
                    white-space: pre-wrap;
                    word-wrap: break-word;
                }
                #status {
                    position: fixed;
                    top: 10px;
                    right: 10px;
                    background: #4CAF50;
                    color: white;
                    padding: 5px 10px;
                    border-radius: 15px;
                    font-size: 12px;
                    animation: pulse 2s infinite;
                }
                @keyframes pulse {
                    0% { opacity: 1; }
                    50% { opacity: 0.6; }
                    100% { opacity: 1; }
                }
            </style>
        </head>
        <body>
            <div id="status">● LIVE</div>
            <div id="preview">Listening...</div>
        </body>
        </html>
    ]])

    previewWindow:show()
end

-- Update preview window with new text
function updatePreview(text)
    if previewWindow then
        -- Escape the text for JavaScript
        local escapedText = text:gsub("\\", "\\\\")
                                :gsub("'", "\\'")
                                :gsub("\n", "\\n")
                                :gsub("\r", "\\r")

        -- Replace entire preview content
        previewWindow:evaluateJavaScript(string.format([[
            document.getElementById('preview').textContent = '%s';
            // Auto-scroll to bottom
            window.scrollTo(0, document.body.scrollHeight);
        ]], escapedText))
    end
end

-- Start recording function
function startRecording()
    print("Starting recording...")

    -- Set recording flag immediately to prevent double-starts
    recording = true

    -- Create preview window
    createPreviewWindow()

    -- Send start command to daemon
    hs.http.post(CLIENT_URL .. "/start", "", {["Content-Type"] = "application/json"}, function(status, body, headers)
        if status == 200 then
            print("Recording started successfully")

            -- Create minimal recording indicator
            local screen = hs.screen.mainScreen():frame()
            indicator = hs.canvas.new({x = screen.x + screen.w - 220, y = 20, w = 200, h = 40})
            indicator[1] = {
                type = "rectangle",
                action = "fill",
                fillColor = {red = 1, green = 0, blue = 0, alpha = 0.5},
                roundedRectRadii = {xRadius = 10, yRadius = 10}
            }
            indicator[2] = {
                type = "text",
                text = "🎙️ Recording...",
                textSize = 16,
                textColor = {white = 1, alpha = 1},
                textAlignment = "center",
                frame = {x = 0, y = 10, w = 200, h = 20}
            }
            indicator:show()

            -- Connect WebSocket for transcriptions
            ws = hs.websocket.new(WS_URL, function(type, message)
                if type == "received" then
                    local msg = hs.json.decode(message)
                    if msg and msg.type == "transcript_final" then
                        local data = hs.json.decode(msg.data)
                        if data and data.text then
                            -- Update preview with full text (replaces everything)
                            updatePreview(data.text)
                        end
                    end
                elseif type == "open" then
                    print("WebSocket connected")
                elseif type == "closed" then
                    print("WebSocket closed")
                end
            end)

            ws:connect()
            -- recording = true  -- Already set at the beginning
        else
            print("Failed to start recording: " .. tostring(status))
            hs.notify.new({title="Recording Failed", informativeText="Could not start recording. Is the daemon running?"}):send()

            -- Reset flag on failure
            recording = false

            if previewWindow then
                previewWindow:delete()
                previewWindow = nil
            end
        end
    end)
end

-- Stop recording function
function stopRecording()
    print("Stopping recording...")

    -- Set recording flag immediately to prevent double-stops
    recording = false

    -- Send stop command to daemon
    hs.http.post(CLIENT_URL .. "/stop", "", {["Content-Type"] = "application/json"}, function(status, body, headers)
        if status == 200 then
            print("Recording stopped successfully")

            -- Get final text from preview
            if previewWindow then
                previewWindow:evaluateJavaScript("document.getElementById('preview').textContent", function(finalText)
                    -- Close preview window
                    previewWindow:delete()
                    previewWindow = nil

                    -- Insert the final text at cursor
                    if finalText and finalText ~= "Listening..." and finalText ~= "" then
                        hs.eventtap.keyStrokes(finalText)
                    end
                end)
            end

            -- Hide indicator
            if indicator then
                indicator:delete()
                indicator = nil
            end

            -- Disconnect WebSocket after a delay to catch final chunks
            if ws then
                hs.timer.doAfter(1, function()
                    ws:close()
                    ws = nil
                end)
            end

            -- recording = false  -- Already set at the beginning
        else
            print("Failed to stop recording: " .. tostring(status))
            -- Reset flag back to true on failure since we're still recording
            recording = true
        end
    end)
end

-- Toggle recording with Ctrl+N
hs.hotkey.bind({"ctrl"}, "n", function()
    if not recording then
        startRecording()
    else
        stopRecording()
    end
end)

print("Hammerspoon transcription with preview window loaded. Press Ctrl+N to toggle recording.")