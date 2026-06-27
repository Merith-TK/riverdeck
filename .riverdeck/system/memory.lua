-- memory.lua - Shows memory usage percentage
-- background() polls the shell every 5s; passive() just reads state (fast).
--
-- passive(key, state) return table fields:
--   color      = {r, g, b}           background fill color (0-255 each); default black
--   icon       = "pkg://pkg#name"    named icon from package registry, composited over color
--              = "./assets/img.png"  path relative to this script's directory
--              = "/path/icon.svg"    path relative to the config root directory
--   text       = "string"            text drawn on top (supports \n for line breaks)
--   text_color = {r, g, b}           text color (default: white {255,255,255})
-- Render order (bottom to top): color -> icon -> text

local shell  = require("shell")
local system = require("system")

local script = {}

function script.background(state)
    while true do
        if system.os() == "windows" then
            local out, _, code = shell.exec("powershell -NoProfile -Command \"$os = Get-CimInstance Win32_OperatingSystem; [math]::Round(($os.TotalVisibleMemorySize - $os.FreePhysicalMemory) / $os.TotalVisibleMemorySize * 100)\"")
            if code == 0 then
                local pct = tonumber((out or ""):match("([%d]+)"))
                if pct then state.memory_percent = pct end
            end
        else
            local out, _, code = shell.exec("free | grep Mem | awk '{printf \"%.0f\", $3/$2 * 100.0}'")
            if code == 0 then
                local pct = tonumber((out or ""):match("([%d]+)"))
                if pct then state.memory_percent = pct end
            end
        end
        system.refresh()
        system.sleep(5000)
    end
end

function script.passive(key, state)
    local pct   = state.memory_percent or 0
    local text  = state.memory_percent and string.format("MEM\n%.0f%%", pct) or "MEM\n--%"
    local color = {0, 255, 0}
    if pct > 90 then
        color = {255, 0, 0}
    elseif pct > 75 then
        color = {255, 165, 0}
    end
    return { color = color, icon = "pkg://riverdeck#memory", text = text, text_color = {255, 255, 255} }
end

return script
