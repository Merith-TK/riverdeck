-- riverdeck.ytmdesktop / buttons/next.lua
--
-- Skips to the next track.  Dims when YTM Desktop is unreachable.

local ytm = require('ytm')

local COL = {
    active  = { bg = {20,  40, 80},  fg = {100, 160, 255} }, -- blue
    offline = { bg = {35,  35, 35},  fg = {100, 100, 100} }, -- grey
}

local M = {}

function M.passive(key, state)
    local col = ytm.connected() and COL.active or COL.offline
    return {
        text       = ">>",
        color      = col.bg,
        text_color = col.fg,
    }
end

function M.trigger(state)
    if not ytm.connected() then return end
    ytm.next()
end

return M
