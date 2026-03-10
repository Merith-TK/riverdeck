-- disk.lua - Shows disk usage for root filesystem
-- background() polls the shell every 30s; passive() just reads state (fast).

local shell  = require("shell")
local system = require("system")

local script = {}

function script.background(state)
    while true do
        if system.os() == "windows" then
            local out, _, code = shell.exec("powershell -NoProfile -Command \"$d = Get-PSDrive C; [math]::Round($d.Used / ($d.Used + $d.Free) * 100)\"")
            if code == 0 then
                local pct = tonumber((out or ""):match("([%d]+)"))
                if pct then state.disk_percent = pct end
            end
        else
            local out, _, code = shell.exec("df / | tail -1 | awk '{print $5}' | sed 's/%//'")
            if code == 0 then
                local pct = tonumber((out or ""):match("([%d]+)"))
                if pct then state.disk_percent = pct end
            end
        end
        system.refresh()
        system.sleep(30000)
    end
end

function script.passive(key, state)
    local pct   = state.disk_percent or 0
    local text  = state.disk_percent and string.format("DISK\n%.0f%%", pct) or "DISK\n--%"
    local color = {0, 255, 0}
    if pct > 95 then
        color = {255, 0, 0}
    elseif pct > 85 then
        color = {255, 165, 0}
    end
    return { color = color, text = text, text_color = {255, 255, 255} }
end

return script
