-- network.lua - Shows network connectivity status
-- background() pings every 10s; passive() just reads state (fast).
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
        return { color = {100, 100, 100}, icon = "pkg://riverdeck#network", text = "NET\n...", text_color = {255, 255, 255} }
    elseif state.network_online then
        return { color = {0, 255, 0}, icon = "pkg://riverdeck#network", text = "NET\nONLINE", text_color = {255, 255, 255} }
    else
        return { color = {255, 0, 0}, icon = "pkg://riverdeck#network", text = "NET\nOFFLINE", text_color = {255, 255, 255} }
    end
end

return script
