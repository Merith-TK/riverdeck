# `time` -- Timestamps and Date Utilities

```lua
local time = require("time")
```

Provides Unix timestamps, date decomposition, formatting, and parsing.

## Functions

### `time.now()`

Current Unix timestamp in seconds.

```lua
local ts = time.now()
```

Returns: `number`

---

### `time.millis()`

Current Unix timestamp in milliseconds. Useful for high-resolution timing.

```lua
local ms = time.millis()
```

Returns: `number`

---

### `time.timestamp()`

Alias for `time.now()`. Kept for backwards compatibility.

---

### `time.date([timestamp])`

Decompose a Unix timestamp (or current time if omitted) into date/time components.

```lua
local d = time.date()
print(d.year, d.month, d.day)
print(d.hour, d.minute, d.second)
print(d.weekday)  -- 0=Sunday, 1=Monday, ..., 6=Saturday
print(d.yearday)  -- 1-365
```

Returns: `table` with fields: `year`, `month`, `day`, `hour`, `minute`, `second`, `weekday`, `yearday`

---

### `time.format(timestamp, layout)`

Format a Unix timestamp using a [Go time layout string](https://pkg.go.dev/time#Layout).

```lua
local str = time.format(time.now(), "2006-01-02 15:04:05")
-- e.g. "2026-05-26 14:30:00"

local clock = time.format(time.now(), "15:04")
-- e.g. "14:30"
```

Common layout tokens:
- `2006` -- 4-digit year
- `01` -- 2-digit month
- `02` -- 2-digit day
- `15` -- 24-hour hour
- `04` -- minute
- `05` -- second

Returns: `string`

---

### `time.parse(layout, value)`

Parse a formatted time string and return a Unix timestamp.

```lua
local ts, err = time.parse("2006-01-02", "2026-05-26")
```

Returns: `number|nil, err|nil`

---

### `time.since(timestamp)`

Seconds elapsed since a Unix timestamp.

```lua
local start = time.now()
-- ... do stuff ...
local elapsed = time.since(start)
```

Returns: `number`

---

### `time.sleep(ms)`

Block for `ms` milliseconds.

> **Warning:** This blocks the goroutine. In `background` functions, use [`system.sleep(ms)`](system.md) instead, which yields cooperatively.

---

## Example: Clock Button

```lua
local time = require("time")

return {
    label = function()
        return time.format(time.now(), "15:04:05")
    end
}
```
