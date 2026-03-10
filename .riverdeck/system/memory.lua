-- memory.lua - Shows memory usage percentage
-- background() polls the shell every 5s; passive() just reads state (fast).

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
    return { color = color, text = text, text_color = {255, 255, 255} }
end

return script
