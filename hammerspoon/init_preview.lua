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

    -- Make it a floating window that stays on top
    previewWindow:windowStyle({"titled", "closable", "resizable"})
    previewWindow:level(hs.drawing.windowLevels.floating)  -- Float above other windows
    previewWindow:windowTitle("Transcription Preview (Live)")
    previewWindow:allowTextEntry(false)  -- No interaction
    previewWindow:bringToFront(true)  -- Bring to front initially

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

    -- Raise to top but don't steal focus
    hs.timer.doAfter(0.1, function()
        if previewWindow then
            local win = previewWindow:hswindow()
            if win then
                win:raise()  -- Bring to top without focusing
            end
        end
    end)
end

-- Update preview window with new text
function updatePreview(text)
    if previewWindow then
        print("Updating preview with text length:", string.len(text))

        -- Use JSON encoding for safe JavaScript string handling
        local jsonText = hs.json.encode(text)

        -- Replace entire preview content
        local js = string.format([[
            try {
                var elem = document.getElementById('preview');
                if (elem) {
                    elem.textContent = %s;
                    // Auto-scroll to bottom
                    window.scrollTo(0, document.body.scrollHeight);
                    console.log('Updated preview with text:', %s);
                } else {
                    console.error('Preview element not found');
                }
            } catch(e) {
                console.error('Error updating preview:', e);
            }
        ]], jsonText, jsonText)

        previewWindow:evaluateJavaScript(js, function(result, error)
            if error then
                print("JavaScript error:", error)
            end
        end)
    else
        print("No preview window to update")
    end
end

-- Start recording function
function startRecording()
    print("Starting recording...")

    -- Set recording flag immediately to prevent double-starts
    recording = true

    -- Create preview window
    createPreviewWindow()

    -- Send start command to daemon (synchronous)
    print("Sending POST to:", CLIENT_URL .. "/start")
    local status, body, headers = hs.http.post(CLIENT_URL .. "/start", "", {["Content-Type"] = "application/json"})
    print("POST response - status:", status, "body:", body)

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
        print("Creating WebSocket to:", WS_URL)
        ws = hs.websocket.new(WS_URL, function(event, message)
            print("WebSocket event:", event, "message:", message)
            if event == "received" then
                local success, data = pcall(hs.json.decode, message)
                if success and data.chunk then
                    print("Transcript chunk length:", string.len(data.chunk))
                    -- Update preview with full text (replaces everything)
                    updatePreview(data.chunk)
                else
                    print("Failed to decode or no chunk in message")
                end
            elseif event == "open" then
                print("WebSocket connected successfully")
            elseif event == "closed" or event == "fail" then
                print("WebSocket closed/failed, event:", event)

                -- WebSocket closed by server (or failed) - time to insert final text
                if not recording then  -- Only if we're in the stopping phase
                    finishRecording()
                end
            end
        end)

        if not ws then
            print("ERROR: Failed to create WebSocket object")
        else
            print("WebSocket object created, waiting for connection...")
        end
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
end

-- Finish recording - called when WebSocket closes (server is done)
function finishRecording()
    print("Finishing recording - inserting final text")

    -- Get final text from preview and insert
    if previewWindow then
        print("Getting final text from preview window")
        previewWindow:evaluateJavaScript("document.getElementById('preview').textContent", function(finalText)
            print("Final text retrieved:", finalText and string.len(finalText) or "nil")

            -- Insert the final text at cursor
            if finalText and finalText ~= "Listening..." and finalText ~= "" then
                print("Inserting final text...")
                hs.eventtap.keyStrokes(finalText)
            end

            -- Close preview window after inserting text
            if previewWindow then
                previewWindow:delete()
                previewWindow = nil
                print("Preview window closed")
            end
        end)
    else
        print("No preview window to close")
    end

    -- Clean up WebSocket reference
    ws = nil
end

-- Stop recording function
function stopRecording()
    print("Stopping recording...")

    -- Set recording flag immediately to prevent double-stops
    recording = false

    -- Send stop command to daemon (synchronous)
    print("Sending POST to:", CLIENT_URL .. "/stop")
    local status, body, headers = hs.http.post(CLIENT_URL .. "/stop", "", {["Content-Type"] = "application/json"})
    print("POST response - status:", status, "body:", body)

    if status == 200 then
        print("Recording stopped successfully - waiting for server to close WebSocket")

        -- Hide indicator immediately
        if indicator then
            indicator:delete()
            indicator = nil
        end

        -- Update preview window title to show it's finalizing
        if previewWindow then
            previewWindow:windowTitle("Transcription Preview (Finalizing...)")
        end

        -- Server will send final updates and then close the WebSocket
        -- finishRecording() will be called when WebSocket closes/fails
    else
        print("Failed to stop recording: " .. tostring(status))
        -- Reset flag back to true on failure since we're still recording
        recording = true
    end
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