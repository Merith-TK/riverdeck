-- riverdeck.ytmdesktop / buttons/like.lua
--
-- Toggles the like on the current track.
--
-- Visual states:
--   ♥  bright pink  Currently liked (like_status == 2)
--   ♡  dim blue     Indifferent / neutral (like_status == 1)
--   👎 orange       Currently disliked (like_status == 0)
--   ♡  grey         YTM Desktop offline / no info

local ytm = require('ytm')

local COL = {
    liked      = { bg = {80,  10,  40},  fg = {255, 80,  140} }, -- pink
    indiff     = { bg = {20,  40,  80},  fg = {100, 160, 255} }, -- blue
    disliked   = { bg = {80,  40,  10},  fg = {255, 160, 60}  }, -- orange
    offline    = { bg = {35,  35,  35},  fg = {100, 100, 100} }, -- grey
}

local M = {}

function M.passive(key, state)
    if not ytm.connected() then
        return {
            text       = "LK",
            color      = COL.offline.bg,
            text_color = COL.offline.fg,
        }
    end

    local ls = ytm.track().like_status

    if ls == 2 then
        return { text = "<3",  color = COL.liked.bg,    text_color = COL.liked.fg    }
    elseif ls == 0 then
        return { text = ":(",  color = COL.disliked.bg, text_color = COL.disliked.fg }
    else
        -- 1 (Indifferent) or -1 (Unknown)
        return { text = "LK",  color = COL.indiff.bg,   text_color = COL.indiff.fg   }
    end
end

function M.trigger(key, state)
    if not ytm.connected() then return end
    local ls = ytm.track().like_status
    -- If disliked, clear it first so we arrive at Indifferent → then Like.
    if ls == 0 then ytm.dislike() end
    ytm.like()
end

return M
