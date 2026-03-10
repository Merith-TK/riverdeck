-- nomad.ytmdesktop / buttons/play_pause.lua
--
-- Toggles playback.  Shows ⏸ when playing, ▶ when paused, and dims when
-- YTM Desktop is unreachable.

local ytm = require('ytm')

local COL = {
    playing = { bg = {10,  60, 20},  fg = {80,  200, 100} }, -- green
    paused  = { bg = {20,  40, 80},  fg = {100, 160, 255} }, -- blue
    offline = { bg = {35,  35, 35},  fg = {100, 100, 100} }, -- grey
}

local M = {}

function M.passive(key, state)
    if not ytm.connected() then
        return {
            text       = ">||",
            color      = COL.offline.bg,
            text_color = COL.offline.fg,
        }
    end

    if ytm.playing() then
        return {
            text       = "||",
            color      = COL.playing.bg,
            text_color = COL.playing.fg,
        }
    else
        return {
            text       = "|>",
            color      = COL.paused.bg,
            text_color = COL.paused.fg,
        }
    end
end

function M.trigger(key, state)
    if not ytm.connected() then return end
    ytm.play_pause()
end

return M
