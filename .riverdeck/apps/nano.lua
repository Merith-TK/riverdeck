-- nano.lua - Opens nano text editor

local shell  = require("shell")
local system = require("system")

RESTART_POLICY = "always"

local script = {}

function script.background(state)
    while true do
        local prev = state.running
        if system.os() == "windows" then
            local out, _, code = shell.exec("tasklist /FI \"IMAGENAME eq nano.exe\" /NH 2>nul")
            state.running = (code == 0 and out:find("nano.exe") ~= nil)
        else
            local _, _, code = shell.exec("pgrep nano >/dev/null 2>&1")
            state.running = (code == 0)
        end
        if state.running ~= prev then
            system.refresh()
        end
        system.sleep(2000)
    end
end

function script.passive(key, state)
    if state.running then
        return { color = {50, 180, 50}, text = "NP*", text_color = {255, 255, 255} }
    else
        return { color = {80, 80, 80},  text = "NP",  text_color = {200, 200, 200} }
    end
end

function script.trigger(state)
    shell.terminal("nano")
    system.refresh()
end

return script
