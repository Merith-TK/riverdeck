-- riverdeck.ytmdesktop / templates/thumbnail.lua
--
-- Shows the current track's thumbnail art.  Falls back to a simple label
-- when YTM Desktop is unreachable.

local M = {}
local ytm = require("ytm")

local COL = {
    offline = { bg = {35, 35, 35}, fg = {100, 100, 100} },
}

function M.passive(key, state)
    if not ytm.connected() then
        return { text = "YTM", color = COL.offline.bg, text_color = COL.offline.fg }
    end
    local trackdata = ytm.track()
    return { image = trackdata.thumbnail, text = "media" }
end

return M