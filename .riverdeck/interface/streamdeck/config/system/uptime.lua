-- uptime.lua - Shows system uptime
-- background() polls the shell every 60s; passive() just reads state (fast).

local shell  = require("shell")
local system = require("system")

local script = {}

function script.background(state)
    while true do
        if system.os() == "windows" then
            local out, _, code = shell.exec("powershell -NoProfile -Command \"((Get-Date) - (gcim Win32_OperatingSystem).LastBootUpTime).TotalSeconds\"")
            if code == 0 then
                local secs = tonumber((out or ""):match("([%d%.]+)")) or 0
                state.uptime_days  = math.floor(secs / 86400)
                state.uptime_hours = math.floor((secs % 86400) / 3600)
                state.uptime_mins  = math.floor((secs % 3600) / 60)
            end
        else
            local out, _, code = shell.exec("uptime -p")
            if code == 0 then
                state.uptime_days  = tonumber((out or ""):match("(%d+) day"))    or 0
                state.uptime_hours = tonumber((out or ""):match("(%d+) hour"))   or 0
                state.uptime_mins  = tonumber((out or ""):match("(%d+) minute")) or 0
            end
        end
        system.refresh()
        system.sleep(60000)
    end
end

function script.passive(key, state)
    if not state.uptime_hours then
        return { color = {0, 100, 200}, text = "UP\n--", text_color = {255, 255, 255} }
    end
    local d, h, m = state.uptime_days or 0, state.uptime_hours or 0, state.uptime_mins or 0
    local text = d > 0
        and string.format("UP\n%dd %dh", d, h)
        or  string.format("UP\n%dh %dm", h, m)
    return { color = {0, 100, 200}, text = text, text_color = {255, 255, 255} }
end

return script
