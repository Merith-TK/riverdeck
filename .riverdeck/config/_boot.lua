-- _boot.lua - Boot animation shown while scripts load
-- Place in config root directory to customize boot sequence
--
-- The boot() function is called once when the interface starts up.
-- This script runs BEFORE other scripts are loaded.

local streamdeck = require("streamdeck")
local time = require("time") -- use time.sleep (blocking), NOT system.sleep (yields coroutine)

local script = {}

function script.boot()
    local cols, rows = streamdeck.get_layout()

    -- Clear all keys
    streamdeck.clear()

    -- Sweep blue from left to right
    for col = 0, cols - 1 do
        for row = 0, rows - 1 do
            local key = row * cols + col
            local intensity = math.floor(100 + (col / cols) * 155)
            streamdeck.set_color(key, 0, 0, intensity)
        end
        time.sleep(100)
    end

    -- Brief pause
    time.sleep(200)

    -- Fade out
    for brightness = 100, 0, -20 do
        streamdeck.set_brightness(brightness)
        time.sleep(50)
    end

    -- Clear and restore brightness
    streamdeck.clear()
    streamdeck.set_brightness(75)
end

return script
