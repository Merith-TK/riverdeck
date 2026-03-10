-- clock.lua - Displays current time on the button

local time = require("time")

local script = {}

function script.passive(key, state)
    local now = time.now()
    if state.last_ts ~= now then
        state.last_ts = now
        local d = time.date(now)
        state.hour = d.hour
        state.min  = d.minute
        state.sec  = d.second
    end

    local h, m, s = state.hour or 0, state.min or 0, state.sec or 0
    -- Blink the separator every other second
    local sep = (s % 2 == 0) and ":" or " "
    local text = string.format("%02d%s%02d", h, sep, m)

    return { color = {20, 20, 60}, text = text, text_color = {100, 200, 255} }
end

return script
