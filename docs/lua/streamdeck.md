# `streamdeck` -- Direct Hardware Control

```lua
local sd = require("streamdeck")
-- or:
local sd = require("riverdeck.streamdeck")
```

Provides direct access to the Stream Deck hardware. Useful for effects or low-level control not covered by the normal script rendering pipeline.

> **Note:** In most cases you do not need this module. Button labels and appearance are handled automatically by the scripting system. Use this module only when you need direct color fills, brightness control, or device info.

## Device Info

### `sd.get_model()`

Returns the device model name string.

```lua
local model = sd.get_model()  -- e.g. "Stream Deck MK.2"
```

Returns: `string|nil`

---

### `sd.get_keys()`

Returns the total number of keys.

```lua
local keys = sd.get_keys()
```

Returns: `number`

---

### `sd.get_layout()`

Returns the number of columns and rows.

```lua
local cols, rows = sd.get_layout()
```

Returns: `number, number`

---

### `sd.device_id()` / `sd.device_serial()`

Returns the device serial number / unique identifier.

```lua
local id = sd.device_id()
```

Returns: `string|nil`

---

## Display Control

### `sd.set_color(key, r, g, b)`

Set a key to a solid RGB color (0-255 each).

```lua
sd.set_color(0, 255, 0, 0)  -- key 0 red
```

Returns: `bool, err|nil`

---

### `sd.clear_key(key)`

Set a single key to black.

```lua
sd.clear_key(3)
```

Returns: `bool, err|nil`

---

### `sd.clear()`

Set all keys to black.

```lua
sd.clear()
```

Returns: `bool, err|nil`

---

### `sd.set_brightness(percent)`

Set the global display brightness (0-100).

```lua
sd.set_brightness(50)
```

Returns: `bool, err|nil`

---

### `sd.reset()`

Reset the device to its factory default state.

```lua
sd.reset()
```

Returns: `bool, err|nil`

---

## Key Numbering

Keys are numbered left-to-right, top-to-bottom starting at 0:

```
MK.2 (5x3):
 0  1  2  3  4
 5  6  7  8  9
10 11 12 13 14
```
