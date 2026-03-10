-- nomad.ytmdesktop / buttons/pair.lua
--
-- Dedicated pairing button for the YTM Desktop companion package.
--
-- Place this script anywhere in your deck layout to manage the YTM connection.
-- The button cycles through all meaningful pairing states visually and is the
-- only way to initiate (or re-initiate) pairing with YTM Desktop.
--
-- Button states (passive display):
--
--   PAIR     red/orange  No token saved.  Press to begin pairing.
--   PAIRING  yellow      Waiting for user to approve in YTM Desktop (≤30s).
--   OFFLINE  grey        Token exists but YTM Desktop is not currently running.
--   REVOKED  orange      YTM Desktop rejected the token (settings reset, etc.).
--                        Press to re-pair.
--   ✓ YTM   dim green   Connected and polling.  Press to force a re-pair.
--
-- Trigger behaviour:
--   - While PAIRING  → no-op (already waiting, nothing to do)
--   - Any other state → calls ytm.request_pair() which signals the daemon

local ytm = require('ytm')

-- -- Colours -------------------------------------------------------------------

local COL = {
    unpaired = { bg = {100, 20,  20},  fg = {255, 120, 120} }, -- dull red
    pairing  = { bg = {100, 80,  0},   fg = {255, 220, 80}  }, -- amber
    offline  = { bg = {40,  40,  40},  fg = {140, 140, 140} }, -- grey
    revoked  = { bg = {110, 50,  0},   fg = {255, 160, 60}  }, -- orange
    connected= { bg = {10,  60,  20},  fg = {80,  200, 100} }, -- green
}

-- -- Passive -------------------------------------------------------------------

local M = {}

function M.passive(key, state)
    -- Initialise blink counter in per-script state.
    state.tick = (state.tick or 0) + 1

    if ytm.pairing() then
        -- Blink the label while waiting for approval.
        local visible = (state.tick % 4) < 2  -- blink at ~0.5 Hz on 2 fps passive
        local label   = visible and "PAIRING" or "..."
        return {
            text       = label,
            color      = COL.pairing.bg,
            text_color = COL.pairing.fg,
        }
    end

    if ytm.connected() then
        return {
            text       = "YTM",
            color      = COL.connected.bg,
            text_color = COL.connected.fg,
        }
    end

    if ytm.revoked() then
        return {
            text       = "REVOKED",
            color      = COL.revoked.bg,
            text_color = COL.revoked.fg,
        }
    end

    if ytm.paired() then
        -- Has a token but YTM Desktop is not responding.
        return {
            text       = "OFFLINE",
            color      = COL.offline.bg,
            text_color = COL.offline.fg,
        }
    end

    -- No token at all.
    return {
        text       = "PAIR",
        color      = COL.unpaired.bg,
        text_color = COL.unpaired.fg,
    }
end

-- -- Trigger -------------------------------------------------------------------

function M.trigger(state)
    -- Ignore presses while a pairing attempt is already in flight.
    if ytm.pairing() then return end

    -- Signal the daemon to start (or restart) the pairing flow.
    ytm.request_pair()
end

return M
