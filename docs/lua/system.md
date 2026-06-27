# `system` -- OS Utilities

```lua
local system = require("system")
-- or:
local system = require("riverdeck.system")
```

Provides OS-level utilities: platform detection, environment variables, cooperative sleep, hostname lookup, and display refresh triggering.

## Functions

### `system.os()`

Returns the current operating system name.

```lua
local os = system.os()  -- "linux", "windows", "darwin"
```

Returns: `string`

---

### `system.env(key)`

Read an environment variable. Returns `nil` if the variable is not set or is empty.

```lua
local home = system.env("HOME")
local api_key = system.env("MY_API_KEY")
```

Returns: `string|nil`

---

### `system.hostname()`

Returns the machine hostname.

```lua
local host = system.hostname()
```

Returns: `string|nil`

---

### `system.sleep(ms)`

Yield the background coroutine for `ms` milliseconds. **Use this in `background` functions** -- it cooperatively yields to Go so other scripts can run.

```lua
return {
    background = function()
        while true do
            -- do work
            system.sleep(1000)  -- wait 1 second
        end
    end
}
```

> **Do not use `time.sleep()` in background functions.** `time.sleep()` blocks the goroutine. `system.sleep()` yields it.

---

### `system.refresh()`

Request an immediate display refresh. Useful in `trigger` to force an instant label update without waiting for the next passive cycle.

```lua
return {
    trigger = function()
        -- change some state
        system.refresh()  -- show the new state immediately
    end
}
```
