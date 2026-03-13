local script = {}
local ytm = require("ytm")

function script.passive(key, state)
   if not ytm.connected() then
	  return { text = "YTM", color = {0, 0, 0}, text_color = {255, 255, 255} }
   end
   local
  trackdata = ytm.track()
  return { image = trackdata.thumbnail, text = "media" }
end

return script