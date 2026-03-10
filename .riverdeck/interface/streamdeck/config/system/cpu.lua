-- cpu.lua - Shows CPU usage percentage
-- background() polls the shell every 5s; passive() just reads state (fast).

local shell  = require("shell")
local system = require("system")

local script = {}

function script.background(state)
    while true do
        local out, _, code
        if system.os() == "windows" then
            -- CIM returns a plain integer e.g. "39"
            out, _, code = shell.exec("powershell -NoProfile -Command (Get-CimInstance Win32_Processor).LoadPercentage")
        else
            out, _, code = shell.exec("top -bn1 | grep 'Cpu(s)' | sed 's/.*, *\\([0-9.]*\\)%* id.*/\\1/' | awk '{print 100 - $1}'")
        end
        if code == 0 then
            local cpu = tonumber((out or ""):match("(%d+)"))
            if cpu then state.cpu = cpu end
        end
        system.refresh()
        system.sleep(5000)
    end
end

function script.passive(key, state)
    local cpu   = state.cpu or 0
    local text  = state.cpu and string.format("CPU\n%.0f%%", cpu) or "CPU\n--%"
    local color = {0, 255, 0}
    if cpu > 80 then
        color = {255, 0, 0}
    elseif cpu > 60 then
        color = {255, 165, 0}
    end
    return { color = color, text = text, text_color = {255, 255, 255} }
end

return script
