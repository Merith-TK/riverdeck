local system = require("system")
local log    = require("log")

local script = {}

local counter = 0

function script.passive(key, state)
    return {
        color = {255, 255, 0},
        text = tostring(counter),
        text_color = {0, 0, 0},
    }
end

function script.trigger(state)
    counter = counter + 1
    log.debug("counter: " .. counter)
    system.refresh()  -- force immediate update
end

return script