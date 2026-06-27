# `store` -- Cross-Script Shared Key-Value Store

```lua
local store = require("store")
-- or:
local store = require("riverdeck.store")
```

A thread-safe global key-value store shared across all running scripts. Values written by one script are immediately visible to all other scripts. Useful for coordinating state between buttons -- for example, a "mode" button that other buttons read to change their behavior.

## Supported Value Types

`string`, `number` (stored as float64), `bool`. Tables and functions cannot be stored.

## Functions

### `store.set(key, value)`

Store a value under a key.

```lua
store.set("obs.streaming", true)
store.set("volume.level", 75)
store.set("mode", "gaming")
```

---

### `store.get(key)`

Retrieve a value. Returns `nil` if the key does not exist.

```lua
local level = store.get("volume.level")
local mode = store.get("mode")
```

Returns: `string|number|bool|nil`

---

### `store.has(key)`

Returns `true` if the key exists.

```lua
if store.has("mode") then ... end
```

Returns: `bool`

---

### `store.delete(key)`

Remove a key. No-op if the key does not exist.

```lua
store.delete("mode")
```

---

### `store.keys()`

Returns a sorted array of all keys currently in the store.

```lua
for _, k in ipairs(store.keys()) do
    print(k, store.get(k))
end
```

Returns: `table` (array of strings)

---

## Example: Mode Toggle

**mode-button.lua** -- toggles a global mode:

```lua
local store = require("store")

return {
    label = function()
        return store.get("mode") == "gaming" and "GAME" or "WORK"
    end,
    trigger = function()
        if store.get("mode") == "gaming" then
            store.set("mode", "work")
        else
            store.set("mode", "gaming")
        end
    end
}
```

**other-button.lua** -- reads the mode:

```lua
local store = require("store")

return {
    label = function()
        local mode = store.get("mode") or "work"
        return mode == "gaming" and "Game App" or "Work App"
    end,
    trigger = function()
        local mode = store.get("mode") or "work"
        if mode == "gaming" then
            shell.exec_async("steam")
        else
            shell.exec_async("slack")
        end
    end
}
```
