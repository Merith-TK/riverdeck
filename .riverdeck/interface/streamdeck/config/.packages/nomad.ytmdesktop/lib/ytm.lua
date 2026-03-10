-- nomad.ytmdesktop / lib/ytm.lua
--
-- Client library for button scripts to interact with YouTube Music Desktop App.
-- Require this module in any button script:
--
--   local ytm = require('ytm')
--
-- The daemon (daemon.lua) keeps the shared store populated with fresh state
-- every ~2 seconds.  Functions in this library read from the store for instant
-- access, avoiding redundant HTTP calls from every button.
--
-- Pairing is initiated by calling ytm.request_pair() from a trigger.
-- The daemon picks up the signal and runs the full pairing flow asynchronously,
-- so the button trigger returns immediately.
--
-- Example pair button script:  see buttons/pair.lua in this package.

local http  = require('http')
local json  = require('json')
local store = require('store')

-- -- Configuration -------------------------------------------------------------

local BASE_URL = "http://127.0.0.1:9863/api/v1"

-- -- Internal helpers ----------------------------------------------------------

-- _token: returns the bearer token from store, or nil if not yet paired.
local function _token()
    return store.get('ytm._token')
end

-- _command: send a POST /command request and return (ok, status).
local function _command(payload)
    local tok = _token()
    if not tok then return false, "not paired" end
    local headers = {
        ["Content-Type"]  = "application/json",
        ["Authorization"] = tok,
    }
    local body, status = http.request("POST", BASE_URL .. "/command", headers, json.encode(payload), 0)
    return (status == 204 or status == 200), tostring(status)
end

-- -- Status queries (read from store - zero latency) ---------------------------

local M = {}

--- Returns true when the daemon has a valid authentication token.
function M.paired()
    return store.has('ytm._token')
end

--- Returns true when YTM Desktop is reachable and the daemon is authenticated.
function M.connected()
    return store.get('ytm.connected') == true
end

--- Returns true while the pairing dialog is open in YTM Desktop.
function M.pairing()
    return store.get('ytm.pairing') == true
end

--- Returns true when the saved token was rejected by YTM Desktop (HTTP 401).
--- The pair button shows a "REVOKED" state in this case.
function M.revoked()
    return store.get('ytm._token_revoked') == true
end

--- Returns true when music is actively playing (trackState == 1).
function M.playing()
    return store.get('ytm.playing') == true
end

--- Returns a table with the current track metadata:
---   title, artist, album, duration (s), progress (s), volume (0-100),
---   track_state (-1/0/1/2), like_status (-1/0/1/2), thumbnail (URL)
function M.track()
    return {
        title       = store.get('ytm.title')       or "",
        artist      = store.get('ytm.artist')      or "",
        album       = store.get('ytm.album')        or "",
        duration    = store.get('ytm.duration')    or 0,
        progress    = store.get('ytm.progress')    or 0,
        volume      = store.get('ytm.volume')      or 0,
        track_state = store.get('ytm.track_state') or -1,
        like_status = store.get('ytm.like_status') or 0,
        thumbnail   = store.get('ytm.thumbnail')   or "",
    }
end

--- Returns the current volume (0-100).
function M.volume()   return store.get('ytm.volume')   or 0  end

--- Returns playback progress in seconds.
function M.progress() return store.get('ytm.progress') or 0  end

--- Returns track duration in seconds.
function M.duration() return store.get('ytm.duration') or 0  end

-- -- Player control (POST /command) -------------------------------------------

--- Send a raw command with optional data payload.
--- See companion API docs for valid command strings.
function M.command(cmd, data)
    local payload = { command = cmd }
    if data ~= nil then payload.data = data end
    return _command(payload)
end

--- Toggle play / pause.
function M.play_pause()   return M.command("playPause")    end

--- Resume playback.
function M.play()         return M.command("play")         end

--- Pause playback.
function M.pause()        return M.command("pause")        end

--- Skip to the next track.
function M.next()         return M.command("next")         end

--- Go back to the previous track.
function M.previous()     return M.command("previous")     end

--- Increase volume by one step.
function M.volume_up()    return M.command("volumeUp")     end

--- Decrease volume by one step.
function M.volume_down()  return M.command("volumeDown")   end

--- Set volume to an exact level (0-100).
function M.set_volume(level)   return M.command("setVolume",      level)   end

--- Mute audio.
function M.mute()         return M.command("mute")         end

--- Unmute audio.
function M.unmute()       return M.command("unmute")       end

--- Seek to a position in the current track (seconds).
function M.seek_to(seconds)    return M.command("seekTo",         seconds) end

--- Toggle shuffle on the current queue.
function M.shuffle()      return M.command("shuffle")      end

--- Set repeat mode: 0 = None, 1 = All, 2 = One.
function M.set_repeat(mode)    return M.command("repeatMode",     mode)    end

--- Jump to a specific queue index.
function M.play_queue(index)   return M.command("playQueueIndex", index)   end

--- Toggle the like on the current track.
function M.like()         return M.command("toggleLike")    end

--- Toggle the dislike on the current track.
function M.dislike()      return M.command("toggleDislike") end

--- Change the current video by videoId and/or playlistId.
--- At least one of videoId or playlistId must be provided.
function M.change_video(video_id, playlist_id)
    return M.command("changeVideo", {
        videoId    = video_id    or json.null,
        playlistId = playlist_id or json.null,
    })
end

-- -- Pairing control -----------------------------------------------------

--- Signal the daemon to start the pairing flow.
--- Returns immediately; the daemon runs the 30-second approval wait on its own
--- goroutine.  Use ytm.pairing() to check progress and ytm.paired() for result.
--- Safe to call even if already paired - it will trigger a fresh re-pairing.
function M.request_pair()
    store.set('ytm._pair_request', true)
end

--- Clear the current token from the store and signal the daemon to forget it.
--- The daemon will delete auth.json on its next loop tick (via the 401 path).
--- After this the pair button must be pressed again to reconnect.
function M.unpair()
    store.delete('ytm._token')
    store.set('ytm.connected',      false)
    store.set('ytm._token_revoked', false)
    -- The daemon detects the missing token naturally on its next poll cycle.
end

return M
