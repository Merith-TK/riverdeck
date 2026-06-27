# `log` -- Levelled Logging

```lua
local log = require("log")
```

Writes messages to stdout with timestamps and log level prefixes. Useful for debugging scripts.

## Functions

### `log.info(message)`

```lua
log.info("script started")
-- output: [SCRIPT] 2026/05/26 14:30:00 [INFO] script started
```

---

### `log.warn(message)`

```lua
log.warn("something looks off")
```

---

### `log.error(message)`

```lua
log.error("failed to connect")
```

---

### `log.debug(message)`

```lua
log.debug("value: " .. tostring(x))
```

---

### `log.print(message)`

Log without a level prefix.

```lua
log.print("raw message")
```

---

### `log.printf(format, ...)`

Formatted log using Go `fmt.Sprintf` syntax.

```lua
log.printf("loaded %d items in %.2f seconds", count, elapsed)
```
