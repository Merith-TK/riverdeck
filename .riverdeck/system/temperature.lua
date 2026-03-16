-- temperature.lua - Shows CPU temperature (if available)
-- background() polls the shell every 10s; passive() just reads state (fast).

local shell  = require("shell")
local system = require("system")

local script = {}

function script.background(state)
    while true do
        if system.os() ~= "windows" then
            -- Try /sys/class/thermal (Linux / Raspberry Pi)
            local out, _, code = shell.exec("cat /sys/class/thermal/thermal_zone0/temp 2>/dev/null || echo '0'")
            if code == 0 then
                local raw = tonumber((out or ""):match("([%d]+)"))
                if raw and raw > 0 then
                    state.temperature = raw / 1000
                end
            end

            -- Fallback: vcgencmd (Raspberry Pi)
            if not state.temperature then
                out, _, code = shell.exec("vcgencmd measure_temp 2>/dev/null | sed 's/temp=//' | sed \"s/'C//\"")
                if code == 0 then
                    state.temperature = tonumber((out or ""):match("([%d%.]+)") or "0")
                end
            end
        else
            -- Windows: temperature not easily available
            state.temperature = 0
        end
        system.refresh()
        system.sleep(10000)
    end
end

function script.passive(key, state)
    local temp  = state.temperature or 0
    local color = {0, 255, 0}
    if temp > 70 then
        color = {255, 0, 0}
    elseif temp > 55 then
        color = {255, 165, 0}
    end
    local display = state.temperature and string.format("%.0f", temp) or "--"
    return { color = color, text = string.format("TEMP\n%s°C", display), text_color = {255, 255, 255} }
end

return script
