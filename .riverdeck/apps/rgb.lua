-- rgb.lua - RGB color cycling icon

local script = {}

function script.passive(key, state)
    state.hue = ((state.hue or 0) + 2) % 360

    local h = state.hue / 60
    local i = math.floor(h)
    local f = h - i
    local q = math.floor(255 * (1 - f))
    local t = math.floor(255 * f)

    local r, g, b = 0, 0, 0
    if i == 0 then r, g, b = 255, t, 0
    elseif i == 1 then r, g, b = q, 255, 0
    elseif i == 2 then r, g, b = 0, 255, t
    elseif i == 3 then r, g, b = 0, q, 255
    elseif i == 4 then r, g, b = t, 0, 255
    else              r, g, b = 255, 0, q
    end

    return { color = {r, g, b}, text = "RGB", text_color = {255, 255, 255} }
end

function script.trigger(state)
    state.hue = 0
end

return script
