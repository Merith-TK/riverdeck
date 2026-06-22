# `mathx` -- Extended Math Utilities

```lua
local mathx = require("mathx")
```

Supplements Lua's built-in `math` library with the three most commonly needed functions it lacks: clamp, round, and lerp.

For everything else (`math.floor`, `math.ceil`, `math.abs`, `math.min`, `math.max`, `math.sin`, `math.cos`, `math.sqrt`, `math.random`, `math.pi`, etc.) use the built-in `math` global directly.

## Functions

### `mathx.clamp(x, lo, hi)`

Constrain `x` to the range `[lo, hi]`.

```lua
local brightness = mathx.clamp(value, 0, 100)
```

Returns: `number`

---

### `mathx.round(x)`

Round to the nearest integer (half away from zero).

```lua
local n = mathx.round(3.7)  -- 4
local n = mathx.round(3.2)  -- 3
```

Returns: `number`

---

### `mathx.lerp(a, b, t)`

Linear interpolation between `a` and `b` by factor `t` in `[0, 1]`.

```lua
local mid = mathx.lerp(0, 100, 0.5)  -- 50
local val = mathx.lerp(10, 20, 0.3)  -- 13
```

Returns: `number`
