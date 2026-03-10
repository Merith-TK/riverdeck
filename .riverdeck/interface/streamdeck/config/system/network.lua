-- network.lua - Shows network connectivity status
-- background() pings every 10s; passive() just reads state (fast).

local shell  = require("shell")
local system = require("system")

local script = {}

function script.background(state)
    while true do
        local cmd
        if system.os() == "windows" then
            cmd = "ping -n 1 -w 1000 8.8.8.8"
        else
            cmd = "ping -c 1 -W 1 8.8.8.8 > /dev/null 2>&1"
        end
        local _, _, code = shell.exec(cmd)
        state.network_online = (code == 0)
        system.refresh()
        system.sleep(10000)
    end
end

function script.passive(key, state)
    if state.network_online == nil then
        -- Not yet polled: show pending state
        return { color = {100, 100, 100}, text = "NET\n...", text_color = {255, 255, 255} }
    elseif state.network_online then
        return { color = {0, 255, 0}, text = "NET\nONLINE", text_color = {255, 255, 255} }
    else
        return { color = {255, 0, 0}, text = "NET\nOFFLINE", text_color = {255, 255, 255} }
    end
end

return script
