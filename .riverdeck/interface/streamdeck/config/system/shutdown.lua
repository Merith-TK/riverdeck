-- shutdown.lua - System shutdown with double-press confirmation
--
-- WARNING: Actual shutdown command commented out for safety

local system = require("system")
local shell  = require("shell")
local time   = require("time")

local script = {}

function script.passive(key, state)
    if state.confirming then
        return { color = {200, 0, 0}, text = "SURE?", text_color = {255, 255, 255} }
    else
        return { color = {100, 30, 30}, text = "OFF", text_color = {200, 200, 200} }
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
