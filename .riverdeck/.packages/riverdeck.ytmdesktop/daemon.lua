-- riverdeck.ytmdesktop / daemon.lua
--
-- Keeps the shared store populated with the current YTM Desktop player state.
-- Polling only starts once a valid token is present.
--
-- Pairing is NOT automatic.  Place the buttons/pair.lua button on your deck
-- and press it to trigger the pairing flow.
--
-- Shared store keys written by this daemon:
--   ytm.connected       bool    YTM Desktop is reachable and authenticated
--   ytm.pairing         bool    True while waiting for user to approve pairing
--   ytm._pair_request   bool    Set to true by the pair button to trigger pairing
--   ytm._token_revoked  bool    True after a 401 (token was rejected / revoked)
--   ytm.playing         bool    True when trackState == 1 (Playing)
--   ytm.track_state     number  -1 Unknown | 0 Paused | 1 Playing | 2 Buffering
--   ytm.title           string  Current track title
--   ytm.artist          string  Current track artist
--   ytm.album           string  Current track album (may be empty)
--   ytm.volume          number  0-100
--   ytm.progress        number  Seconds elapsed
--   ytm.duration        number  Track length in seconds
--   ytm.like_status     number  -1 Unknown | 0 Dislike | 1 Indifferent | 2 Like
--   ytm.thumbnail       string  URL of the best-resolution thumbnail
--   ytm._token          string  Bearer token (private; used by lib/ytm.lua)

local http     = require('http')
local json     = require('json')
local store    = require('store')
local pkg_data = require('pkg_data')
local system   = require('system')
local log      = require('log')

-- -- Configuration -------------------------------------------------------------

local BASE_URL      = "http://127.0.0.1:9863/api/v1"
local APP_ID        = "riverdeckstreamdeck"
local APP_NAME      = "Riverdeck Stream Deck"
local APP_VER       = "1.0.0"
local POLL_INTERVAL = 2000   -- ms between state polls when connected
local RETRY_DELAY   = 10000  -- ms between reconnect attempts when YTM is absent
local PAIR_TIMEOUT  = 35000  -- ms to wait for user to approve pairing (API max 30s)

-- -- HTTP helpers --------------------------------------------------------------

-- post_json: POST a JSON-encoded payload; returns (table|nil, status_code).
local function post_json(url, payload, token, timeout_ms)
    local headers = { ["Content-Type"] = "application/json" }
    if token then headers["Authorization"] = token end
    local body, status = http.request("POST", url, headers, json.encode(payload), timeout_ms or 0)
    if not body then
        -- body is nil only on a hard network error; status contains the Go error string.
        log.error("[ytm] HTTP error: " .. tostring(status))
        return nil, 0
    end
    local t, decode_err = json.decode(body)
    if not t then
        log.error("[ytm] JSON decode failed (" .. tostring(decode_err) .. ") for body: " .. string.sub(tostring(body), 1, 200))
        return nil, status
    end
    return t, status
end

-- get_json: GET with optional auth header; returns (table|nil, status_code).
-- status_code is 0 on hard network error, -1 on successful HTTP but JSON parse
-- failure (to distinguish from real connection problems).
local function get_json(url, token)
    local headers = {}
    if token then headers["Authorization"] = token end
    local body, status = http.request("GET", url, headers, "", 0)
    if not body then return nil, 0 end
    local t, decode_err = json.decode(body)
    if not t then
        log.error("[ytm] GET JSON decode failed (" .. tostring(decode_err) .. ") body: " .. string.sub(tostring(body), 1, 200))
        return nil, -1  -- special code: HTTP worked but parse failed
    end
    return t, status
end

-- -- Pairing -------------------------------------------------------------------

-- do_pair: run the full requestcode -> request authentication flow.
-- Only called when the pair button sets ytm._pair_request = true.
-- Returns the token string on success, or nil on failure.
local function do_pair()
    store.set('ytm.pairing', true)
    log.info("[ytm] Starting pairing flow...")

    -- Step 1 - request a one-time code
    local res, status = post_json(BASE_URL .. "/auth/requestcode", {
        appId      = APP_ID,
        appName    = APP_NAME,
        appVersion = APP_VER,
    })
    if not res or not res.code then
        log.error("[ytm] Failed to obtain pairing code (is YTM Desktop running?). HTTP " .. tostring(status))
        store.set('ytm.pairing', false)
        return nil
    end

    local code = res.code
    log.info("[ytm] ┌---------------------------------------------------------┐")
    log.info("[ytm] │  YTM Desktop pairing request sent.                      │")
    log.info("[ytm] │  Open YTM Desktop -> Settings -> Integrations             │")
    log.info("[ytm] │  and APPROVE the connection for \"" .. APP_NAME .. "\".  │")
    log.info("[ytm] │  You have 30 seconds.                                   │")
    log.info("[ytm] └---------------------------------------------------------┘")

    -- Step 2 - exchange code for token (blocks up to PAIR_TIMEOUT)
    local auth, auth_status = post_json(
        BASE_URL .. "/auth/request",
        { appId = APP_ID, code = code },
        nil,
        PAIR_TIMEOUT
    )

	log.debug("[ytm] Auth response: " .. json.encode(auth) .. ", HTTP " .. tostring(auth_status))

    store.set('ytm.pairing', false)

    if not auth or not auth.token then
        log.error("[ytm] Pairing failed - HTTP " .. tostring(auth_status)
            .. ". Response token field: " .. tostring(auth and auth.token))
        return nil
    end

    log.info("[ytm] Paired successfully!")
    return auth.token
end

-- -- State handling ------------------------------------------------------------

-- push_state: writes fields from a /state response into the shared store.
local function push_state(s)
    if not s then return end

    store.set('ytm.connected', true)

    local player = s.player
    if player then
        store.set('ytm.track_state', player.trackState    or -1)
        store.set('ytm.playing',     player.trackState    == 1)
        store.set('ytm.volume',      player.volume        or 0)
        store.set('ytm.progress',    player.videoProgress or 0)
    end

    local video = s.video
    if video then
        store.set('ytm.title',       video.title          or "")
        store.set('ytm.artist',      video.author         or "")
        store.set('ytm.album',       video.album          or "")
        store.set('ytm.duration',    video.durationSeconds or 0)
        store.set('ytm.like_status', video.likeStatus     or -1)
        -- Pick the largest available thumbnail (last entry in array).
        local thumbs = video.thumbnails
        if thumbs and #thumbs > 0 then
            store.set('ytm.thumbnail', thumbs[#thumbs].url or "")
        end
    else
        -- YTM is open but nothing is loaded / playing.
        store.set('ytm.title',       "")
        store.set('ytm.artist',      "")
        store.set('ytm.album',       "")
        store.set('ytm.playing',     false)
        store.set('ytm.track_state', -1)
        store.set('ytm.thumbnail',   "")
    end
end

-- -- Daemon entry point --------------------------------------------------------

local M = {}

function M.daemon(state)
    -- Seed the store with safe defaults so buttons never read nil.
    store.set('ytm.connected',      false)
    store.set('ytm.pairing',        false)
    store.set('ytm._pair_request',  false)
    store.set('ytm._token_revoked', false)
    store.set('ytm.playing',        false)
    store.set('ytm.track_state',    -1)
    store.set('ytm.title',          "")
    store.set('ytm.artist',         "")
    store.set('ytm.album',          "")
    store.set('ytm.volume',         0)
    store.set('ytm.progress',       0)
    store.set('ytm.duration',       0)
    store.set('ytm.like_status',    -1)
    store.set('ytm.thumbnail',      "")

    -- Load previously saved token from package-private data storage.
    local token = nil
    local auth = pkg_data.json_read('auth.json')
    if auth and auth.token then
        token = auth.token
        store.set('ytm._token', token)
        log.info("[ytm] Loaded saved authentication token.")
    else
        log.info("[ytm] No saved token - press the PAIR button on your deck to connect.")
    end

    -- -- Main loop --------------------------------------------------------------
    while true do

        -- Check for a pairing signal from the pair button.
        if store.get('ytm._pair_request') == true then
            store.set('ytm._pair_request',  false) -- consume signal immediately
            store.set('ytm._token_revoked', false) -- clear any previous revocation
            token = do_pair()
            if token then
                pkg_data.json_write('auth.json', { token = token })
                store.set('ytm._token', token)
            end
            system.sleep(500) -- brief pause before first poll
        end

        if token then
            -- Poll the current player state.
            local s, status = get_json(BASE_URL .. "/state", token)

            if status == 401 then
                -- Token was revoked or is no longer valid in YTM Desktop.
                log.warn("[ytm] Token rejected (HTTP 401) - cleared. Press PAIR to reconnect.")
                token = nil
                store.delete('ytm._token')
                pkg_data.remove('auth.json')
                store.set('ytm.connected',      false)
                store.set('ytm._token_revoked', true)

            elseif status == 200 and s then
                store.set('ytm._token_revoked', false)
                push_state(s)

            elseif status == -1 then
                -- HTTP worked but we couldn't parse the response - don't
                -- treat this as a full disconnect; just retry at normal rate.
                log.warn("[ytm] /state parse error - retrying next poll.")

            else
                -- YTM Desktop stopped or network hiccup.
                store.set('ytm.connected', false)
                system.sleep(RETRY_DELAY)
            end

            system.sleep(POLL_INTERVAL)
        else
            -- No token and no incoming pair request - sleep briefly then re-check.
            system.sleep(1000)
        end

    end -- while true
end

return M
