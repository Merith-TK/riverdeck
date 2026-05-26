-- shutdown.lua - System shutdown with double-press confirmation
--
-- WARNING: Actual shutdown command commented out for safety
--
-- passive(key, state) return table fields:
--   color      = {r, g, b}           background fill color (0-255 each); default black
--   icon       = "pkg://pkg#name"    named icon from package registry, composited over color
--              = "./assets/img.png"  path relative to this script's directory
--              = "/path/icon.svg"    path relative to the config root directory
--   text       = "string"            text drawn on top (supports \n for line breaks)
--   text_color = {r, g, b}           text color (default: white {255,255,255})
-- Render order (bottom to top): color -> icon -> text

local system = require("system")
local shell  = require("shell")
local time   = require("time")

local script = {}

function script.passive(key, state)
    if state.confirming then
        return { color = {200, 0, 0}, icon = "pkg://riverdeck#shutdown", text = "SURE?", text_color = {255, 255, 255} }
    else
        return { color = {100, 30, 30}, icon = "pkg://riverdeck#shutdown", text = "OFF", text_color = {200, 200, 200} }
    end
end

function script.background(state)
    while true do
        if state.confirming then
            if (time.now() - (state.confirm_time or 0)) > 3 then
                state.confirming = false
                system.refresh()
            end
        end
        system.sleep(500)
    end
end

function script.trigger(state)
    if state.confirming then
        state.confirming = false
        -- Uncomment to enable actual shutdown:
        -- if system.os() == "windows" then
        --     shell.exec("shutdown /s /t 60 /c \"Shutdown initiated from Stream Deck\"")
        -- else
        --     shell.exec("shutdown -h now")
        -- end
    else
        state.confirming   = true
        state.confirm_time = time.now()
    end
    system.refresh()
end

return script
