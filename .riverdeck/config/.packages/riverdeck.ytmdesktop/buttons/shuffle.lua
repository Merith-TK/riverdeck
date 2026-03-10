-- riverdeck.ytmdesktop / buttons/shuffle.lua
--
-- Sends a shuffle toggle command.  The YTM API does not expose the current
-- shuffle state so the button flashes briefly on press to indicate the command
-- was sent, then returns to its resting colour.

local ytm = require('ytm')

local COL = {
    active  = { bg = {30,  20,  80},  fg = {160, 120, 255} }, -- purple
    flash   = { bg = {80,  60,  160}, fg = {220, 200, 255} }, -- bright purple
    offline = { bg = {35,  35,  35},  fg = {100, 100, 100} }, -- grey
}

local M = {}

function M.passive(key, state)
    -- Flash for 2 passive ticks (~1 s) after a press, then return to normal.
    if state.flash and state.flash > 0 then
        state.flash = state.flash - 1
        return {
            text       = "><",
            color      = COL.flash.bg,
            text_color = COL.flash.fg,
        }
    end

    local col = ytm.connected() and COL.active or COL.offline
    return {
        text       = "><",
        color      = col.bg,
        text_color = col.fg,
    }
end

function M.trigger(key, state)
    if not ytm.connected() then return end
    ytm.shuffle()
    state.flash = 2  -- arm the flash
end

return M
