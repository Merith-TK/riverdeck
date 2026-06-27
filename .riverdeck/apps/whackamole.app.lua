local app = require('app')
local sd  = require('streamdeck')

local M = {}

-- Pick a random mole slot, avoiding score_slot and the current mole position.
local function pick_mole(current, score_slot, total)
    if total <= 1 then return 0 end
    local candidate
    repeat
        local idx = math.random(0, total - 2)  -- pool excludes one slot (score)
        if idx >= score_slot then idx = idx + 1 end
        candidate = idx
    until candidate ~= current
    return candidate
end

-- Lazy initialisation: runs once on the first passive tick.
local function ensure_init(state)
    if state.initialized then return end
    local cols, rows = sd.get_layout()
    state.cols       = cols
    state.rows       = rows
    state.score_slot = cols - 1          -- top-right key
    state.total      = cols * rows
    state.score      = 0
    state.mole       = pick_mole(-1, state.score_slot, state.total)
    state.initialized = true
end

function M.app_passive(key, state)
    ensure_init(state)

    -- Top-right: score display + tap to exit hint
    if key == state.score_slot then
        return {
            color      = {20, 20, 80},
            text       = tostring(state.score) .. "\n[exit]",
            text_color = {180, 180, 255},
        }
    end

    -- The active mole
    if key == state.mole then
        return {
            color      = {30, 220, 30},
            text       = "!",
            text_color = {0, 0, 0},
        }
    end

    -- Empty key
    return {color = {10, 10, 10}}
end

function M.app_key(key, state)
    ensure_init(state)

    -- Score key doubles as exit button
    if key == state.score_slot then
        app.exit()
        return
    end

    -- Hit the mole
    if key == state.mole then
        state.score = state.score + 1
        state.mole  = pick_mole(state.mole, state.score_slot, state.total)
    end
    -- Missed: silently ignore
end

return M
