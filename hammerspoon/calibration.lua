-- VAD Calibration UI for Hammerspoon
-- Provides a visual wizard for calibrating voice activity detection

local calibration = {}

-- Configuration
local config = {
    daemonURL = "http://localhost:8081",
    recordDuration = 5, -- seconds
}

-- State
local state = {
    window = nil,
    currentStep = nil,
    backgroundStats = nil,
    speechStats = nil,
    recommendedThreshold = nil,
    isRecording = false,
}

-- HTTP helper
local function httpRequest(method, path, body, callback)
    local url = config.daemonURL .. path
    local headers = {}
    local bodyData = nil

    if body then
        headers["Content-Type"] = "application/json"
        bodyData = hs.json.encode(body)
    end

    hs.http.doAsyncRequest(url, method, bodyData, headers, function(status, responseBody, headers)
        if callback then
            local success, data = pcall(hs.json.decode, responseBody)
            if success then
                callback(status, data)
            else
                callback(status, {error = responseBody})
            end
        end
    end)
end

-- Create calibration window
local function createWindow()
    local mainScreen = hs.screen.mainScreen()
    local frame = mainScreen:frame()

    -- Center window (500x400)
    local width = 500
    local height = 400
    local x = frame.x + (frame.w - width) / 2
    local y = frame.y + (frame.h - height) / 2

    local canvas = hs.canvas.new({x = x, y = y, w = width, h = height})
    canvas:level("floating")
    canvas:behavior(hs.canvas.windowBehaviors.canJoinAllSpaces)

    -- Background
    canvas:appendElements({
        type = "rectangle",
        action = "fill",
        fillColor = {red = 0.15, green = 0.15, blue = 0.15, alpha = 0.95},
        roundedRectRadii = {xRadius = 12, yRadius = 12},
    })

    return canvas
end

-- Draw step 1: Background recording
local function drawStep1()
    state.window:replaceElements({
        -- Background
        {
            type = "rectangle",
            action = "fill",
            fillColor = {red = 0.15, green = 0.15, blue = 0.15, alpha = 0.95},
            roundedRectRadii = {xRadius = 12, yRadius = 12},
        },
        -- Title
        {
            type = "text",
            text = "🎤 VAD Calibration - Step 1/3",
            textSize = 20,
            textColor = {white = 1, alpha = 1},
            textAlignment = "center",
            frame = {x = 20, y = 20, w = 460, h = 30},
        },
        -- Subtitle
        {
            type = "text",
            text = "Background Noise",
            textSize = 24,
            textColor = {red = 0.4, green = 0.8, blue = 1.0, alpha = 1},
            textAlignment = "center",
            frame = {x = 20, y = 55, w = 460, h = 35},
        },
        -- Instructions
        {
            type = "text",
            text = "Stay completely silent.\nDon't speak or move.\n\nThis measures ambient noise in your environment.",
            textSize = 16,
            textColor = {white = 0.9, alpha = 1},
            textAlignment = "center",
            frame = {x = 40, y = 120, w = 420, h = 100},
        },
        -- Button background (if not recording)
        state.isRecording and {} or {
            type = "rectangle",
            action = "fill",
            fillColor = {red = 0.2, green = 0.6, blue = 1.0, alpha = 1.0},
            roundedRectRadii = {xRadius = 8, yRadius = 8},
            frame = {x = 175, y = 300, w = 150, h = 50},
        },
        -- Button text
        {
            type = "text",
            text = state.isRecording and ("Recording... " .. math.floor(state.recordProgress or 0) .. "s") or "Start Recording",
            textSize = 18,
            textColor = {white = 1, alpha = 1},
            textAlignment = "center",
            frame = {x = 175, y = 312, w = 150, h = 30},
        },
    })
end

-- Draw step 2: Speech recording
local function drawStep2()
    state.window:replaceElements({
        -- Background
        {
            type = "rectangle",
            action = "fill",
            fillColor = {red = 0.15, green = 0.15, blue = 0.15, alpha = 0.95},
            roundedRectRadii = {xRadius = 12, yRadius = 12},
        },
        -- Title
        {
            type = "text",
            text = "🎤 VAD Calibration - Step 2/3",
            textSize = 20,
            textColor = {white = 1, alpha = 1},
            textAlignment = "center",
            frame = {x = 20, y = 20, w = 460, h = 30},
        },
        -- Subtitle
        {
            type = "text",
            text = "Speech Recording",
            textSize = 24,
            textColor = {red = 1.0, green = 0.6, blue = 0.4, alpha = 1},
            textAlignment = "center",
            frame = {x = 20, y = 55, w = 460, h = 35},
        },
        -- Instructions
        {
            type = "text",
            text = "Speak normally and continuously.\nUse your natural speaking voice.\n\nSay anything - the content doesn't matter.",
            textSize = 16,
            textColor = {white = 0.9, alpha = 1},
            textAlignment = "center",
            frame = {x = 40, y = 120, w = 420, h = 100},
        },
        -- Background stats (show what we measured)
        {
            type = "text",
            text = string.format("✓ Background measured\n   Avg: %.1f  |  P95: %.1f",
                state.backgroundStats.avg, state.backgroundStats.p95),
            textSize = 12,
            textColor = {white = 0.6, alpha = 1},
            textAlignment = "center",
            frame = {x = 40, y = 230, w = 420, h = 40},
        },
        -- Button background (if not recording)
        state.isRecording and {} or {
            type = "rectangle",
            action = "fill",
            fillColor = {red = 0.2, green = 0.6, blue = 1.0, alpha = 1.0},
            roundedRectRadii = {xRadius = 8, yRadius = 8},
            frame = {x = 175, y = 300, w = 150, h = 50},
        },
        -- Button text
        {
            type = "text",
            text = state.isRecording and ("Recording... " .. math.floor(state.recordProgress or 0) .. "s") or "Start Recording",
            textSize = 18,
            textColor = {white = 1, alpha = 1},
            textAlignment = "center",
            frame = {x = 175, y = 312, w = 150, h = 30},
        },
    })
end

-- Draw step 3: Results and save
local function drawStep3()
    local elements = {
        -- Background
        {
            type = "rectangle",
            action = "fill",
            fillColor = {red = 0.15, green = 0.15, blue = 0.15, alpha = 0.95},
            roundedRectRadii = {xRadius = 12, yRadius = 12},
        },
        -- Title
        {
            type = "text",
            text = "🎤 VAD Calibration - Step 3/3",
            textSize = 20,
            textColor = {white = 1, alpha = 1},
            textAlignment = "center",
            frame = {x = 20, y = 20, w = 460, h = 30},
        },
        -- Subtitle
        {
            type = "text",
            text = "Results",
            textSize = 24,
            textColor = {red = 0.4, green = 1.0, blue = 0.6, alpha = 1},
            textAlignment = "center",
            frame = {x = 20, y = 55, w = 460, h = 35},
        },
        -- Stats comparison
        {
            type = "text",
            text = string.format("Background: Avg %.1f  |  P95 %.1f",
                state.backgroundStats.avg, state.backgroundStats.p95),
            textSize = 13,
            textColor = {white = 0.8, alpha = 1},
            textAlignment = "left",
            frame = {x = 40, y = 110, w = 420, h = 20},
        },
        {
            type = "text",
            text = string.format("Speech: Avg %.1f  |  P5 %.1f",
                state.speechStats.avg, state.speechStats.p5),
            textSize = 13,
            textColor = {white = 0.8, alpha = 1},
            textAlignment = "left",
            frame = {x = 40, y = 135, w = 420, h = 20},
        },
    }

    -- Visual bars
    local maxVal = math.max(state.backgroundStats.avg, state.speechStats.avg) * 1.2
    if maxVal == 0 then maxVal = 1 end

    local bgBarWidth = (state.backgroundStats.avg / maxVal) * 350
    local speechBarWidth = (state.speechStats.avg / maxVal) * 350

    table.insert(elements, {
        type = "rectangle",
        action = "fill",
        fillColor = {red = 0.4, green = 0.8, blue = 1.0, alpha = 0.6},
        frame = {x = 70, y = 170, w = bgBarWidth, h = 20},
    })

    table.insert(elements, {
        type = "rectangle",
        action = "fill",
        fillColor = {red = 1.0, green = 0.6, blue = 0.4, alpha = 0.6},
        frame = {x = 70, y = 200, w = speechBarWidth, h = 20},
    })

    -- Recommended threshold
    table.insert(elements, {
        type = "text",
        text = string.format("📊 Recommended Threshold: %.0f", state.recommendedThreshold),
        textSize = 18,
        textColor = {red = 0.4, green = 1.0, blue = 0.6, alpha = 1},
        textAlignment = "center",
        frame = {x = 40, y = 240, w = 420, h = 30},
    })

    table.insert(elements, {
        type = "text",
        text = string.format("~%d%% background / ~%d%% speech detected",
            state.backgroundFramesAbove or 5, state.speechFramesAbove or 95),
        textSize = 12,
        textColor = {white = 0.7, alpha = 1},
        textAlignment = "center",
        frame = {x = 40, y = 270, w = 420, h = 20},
    })

    -- Save button
    table.insert(elements, {
        type = "rectangle",
        action = "fill",
        fillColor = {red = 0.2, green = 0.8, blue = 0.4, alpha = 1.0},
        roundedRectRadii = {xRadius = 8, yRadius = 8},
        frame = {x = 100, y = 320, w = 120, h = 50},
    })

    table.insert(elements, {
        type = "text",
        text = "Save & Close",
        textSize = 16,
        textColor = {white = 1, alpha = 1},
        textAlignment = "center",
        frame = {x = 100, y = 333, w = 120, h = 30},
    })

    -- Cancel button
    table.insert(elements, {
        type = "rectangle",
        action = "fill",
        fillColor = {red = 0.6, green = 0.2, blue = 0.2, alpha = 1.0},
        roundedRectRadii = {xRadius = 8, yRadius = 8},
        frame = {x = 280, y = 320, w = 120, h = 50},
    })

    table.insert(elements, {
        type = "text",
        text = "Cancel",
        textSize = 16,
        textColor = {white = 1, alpha = 1},
        textAlignment = "center",
        frame = {x = 280, y = 333, w = 120, h = 30},
    })

    state.window:replaceElements(elements)
end

-- Record audio for calibration
local function recordAudio(callback)
    state.isRecording = true
    state.recordProgress = 0

    -- Update UI every 0.5s
    local timer = hs.timer.doEvery(0.5, function()
        state.recordProgress = state.recordProgress + 0.5
        if state.currentStep == 1 then
            drawStep1()
        elseif state.currentStep == 2 then
            drawStep2()
        end
    end)

    -- Make recording request
    httpRequest("POST", "/api/calibrate/record", {duration_seconds = config.recordDuration}, function(status, data)
        timer:stop()
        state.isRecording = false

        if status == 200 and data then
            callback(data)
        else
            hs.notify.new({
                title = "Calibration Error",
                informativeText = "Failed to record audio: " .. (data.error or "Unknown error")
            }):send()
            calibration.close()
        end
    end)
end

-- Handle button clicks
local function handleClick(x, y)
    if state.currentStep == 1 and not state.isRecording then
        -- Start background recording
        if x >= 175 and x <= 325 and y >= 300 and y <= 350 then
            recordAudio(function(stats)
                state.backgroundStats = stats
                state.currentStep = 2
                drawStep2()
            end)
        end
    elseif state.currentStep == 2 and not state.isRecording then
        -- Start speech recording
        if x >= 175 and x <= 325 and y >= 300 and y <= 350 then
            recordAudio(function(stats)
                state.speechStats = stats

                -- Calculate threshold
                httpRequest("POST", "/api/calibrate/calculate", {
                    background = state.backgroundStats,
                    speech = state.speechStats
                }, function(status, data)
                    if status == 200 and data then
                        state.recommendedThreshold = data.threshold
                        state.backgroundFramesAbove = data.background_frames_above_percent
                        state.speechFramesAbove = data.speech_frames_above_percent
                        state.currentStep = 3
                        drawStep3()
                    else
                        hs.notify.new({
                            title = "Calibration Error",
                            informativeText = "Failed to calculate threshold"
                        }):send()
                        calibration.close()
                    end
                end)
            end)
        end
    elseif state.currentStep == 3 then
        -- Save button (100-220, 320-370)
        if x >= 100 and x <= 220 and y >= 320 and y <= 370 then
            httpRequest("POST", "/api/calibrate/save", {
                threshold = state.recommendedThreshold
            }, function(status, data)
                if status == 200 and data.success then
                    hs.notify.new({
                        title = "Calibration Complete",
                        informativeText = string.format("Threshold %.0f saved to config", state.recommendedThreshold)
                    }):send()
                    calibration.close()
                else
                    hs.notify.new({
                        title = "Save Failed",
                        informativeText = "Failed to save config"
                    }):send()
                end
            end)
        end
        -- Cancel button (280-400, 320-370)
        if x >= 280 and x <= 400 and y >= 320 and y <= 370 then
            calibration.close()
        end
    end
end

-- Public API

function calibration.show()
    if state.window then
        calibration.close()
    end

    -- Reset state
    state.currentStep = 1
    state.backgroundStats = nil
    state.speechStats = nil
    state.recommendedThreshold = nil
    state.isRecording = false

    -- Create window
    state.window = createWindow()

    -- Setup click handler
    state.window:clickActivating(false)
    state.window:canvasMouseEvents(true, true)
        :mouseCallback(function(canvas, event, id, x, y)
            if event == "mouseDown" then
                handleClick(x, y)
            end
        end)

    -- Draw initial step
    drawStep1()

    -- Show window
    state.window:show()
end

function calibration.close()
    if state.window then
        state.window:delete()
        state.window = nil
    end
    state.currentStep = nil
end

return calibration
