--[[
  example_module.lua
  Demonstrates the Riverdeck module-based scripting architecture.

  Every script must return a table containing any combination of:
    script.background(state)         -- coroutine; use system.sleep() to yield
    script.passive(key, state)       -- called at passive FPS; return appearance
    script.trigger(state)            -- called on key press

  The `state` table persists across all calls for this script.
  Use it to share data between background, passive, and trigger.

  Available modules (require them at the top):
    shell      - exec, exec_async, open, terminal
    system     - os, env, hostname, sleep (background only), refresh
    http       - get, post, request
    streamdeck - set_color, set_brightness, clear, get_layout, get_keys
    file       - read, write, append, exists, list, is_dir, size, remove, mkdir
    time       - now, date, format, parse, sleep (safe anywhere)
    json       - encode, decode
    log        - info, warn, error, debug, printf
    utils      - deepcopy, contains, size, merge
    strings    - split, trim, startswith, endswith, replace, upper, lower, ...
    store      - get, set, delete, has, keys  (shared across ALL scripts)
]]

local system = require("system")
local log    = require("log")

local script = {}

-- Module-local state (shared across all three functions for THIS script).
-- You can also use the `state` argument directly; the difference is that
-- locals like these are reset if the script runner is restarted, while
-- values written to `state` survive background restarts (same Lua state).
local click_count = 0
local color       = {255, 0, 255} -- Magenta

--[[
  script.background(state)
  Runs as a Lua coroutine managed by the Go runner.
  Use `system.sleep(ms)` to yield — passive() and trigger() execute during
  the sleep window.  Do NOT use time.sleep() here; that blocks the goroutine.
  This coroutine restarts according to RESTART_POLICY if it exits or errors.
]]
function script.background(state)
    while true do
        -- Log a heartbeat so you can verify background is running
        log.debug("background tick, clicks=" .. click_count)
        system.sleep(5000)  -- yield for 5 seconds
    end
end

--[[
  script.passive(key, state) -> table|nil
  Called at the passive FPS (default 30 fps) while the key is on-screen.
  Return an appearance table to update the key, or nil to leave it unchanged.

  Appearance fields (all optional):
    color      = {r, g, b}     -- background fill (0-255 each)
    text       = "string"      -- label; \n for multi-line
    text_color = {r, g, b}     -- label colour (default: white)
    image      = "path"        -- relative .png/.jpg path; overrides color/text

  Keep passive() fast — never do shell/http/file I/O here.
  Use background() to fetch data, store it in state, and call system.refresh().
]]
function script.passive(key, state)
    return {
        color      = color,
        text       = tostring(click_count),
        text_color = {255, 255, 255},
    }
end

--[[
  script.trigger(state)
  Called once when the physical key is pressed.
  Avoid long blocking operations; for slow work, set a flag in state and
  handle it in background(), or use shell.exec_async().
  Call system.refresh() to force an immediate passive redraw after state changes.
]]
function script.trigger(state)
    click_count = click_count + 1
    -- Toggle colour on every press
    color = click_count % 2 == 0 and {0, 255, 0} or {255, 0, 255}
    log.info("triggered, count=" .. click_count)
    system.refresh()  -- force an immediate passive redraw
end

return script

