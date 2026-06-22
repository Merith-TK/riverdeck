# `strings` -- String Utilities

```lua
local strings = require("strings")
```

String manipulation functions backed by Go's `strings` package.

## Functions

### `strings.split(str, sep)`

Split a string by a separator. Returns an array table.

```lua
local parts = strings.split("a,b,c", ",")
-- parts = {"a", "b", "c"}
```

---

### `strings.join(tbl, sep)`

Join an array table into a string with a separator.

```lua
local str = strings.join({"a", "b", "c"}, "-")
-- str = "a-b-c"
```

---

### `strings.trim(str)`

Remove leading and trailing whitespace.

```lua
local s = strings.trim("  hello  ")  -- "hello"
```

---

### `strings.trim_prefix(str, prefix)`

Remove a leading prefix if present.

```lua
local s = strings.trim_prefix("foobar", "foo")  -- "bar"
```

---

### `strings.trim_suffix(str, suffix)`

Remove a trailing suffix if present.

```lua
local s = strings.trim_suffix("foobar", "bar")  -- "foo"
```

---

### `strings.startswith(str, prefix)`

Returns `true` if `str` starts with `prefix`.

```lua
if strings.startswith(path, "/home") then ... end
```

---

### `strings.endswith(str, suffix)`

Returns `true` if `str` ends with `suffix`.

```lua
if strings.endswith(file, ".lua") then ... end
```

---

### `strings.contains(str, substr)`

Returns `true` if `substr` is found in `str`.

```lua
if strings.contains(output, "error") then ... end
```

---

### `strings.index(str, substr)`

Returns the 1-based position of the first occurrence of `substr`, or `0` if not found.

```lua
local pos = strings.index("hello world", "world")  -- 7
```

---

### `strings.count(str, substr)`

Count non-overlapping occurrences of `substr` in `str`.

```lua
local n = strings.count("banana", "an")  -- 2
```

---

### `strings.replace(str, old, new [, count])`

Replace occurrences of `old` with `new`. Pass `-1` (or omit `count`) to replace all.

```lua
local s = strings.replace("aabbcc", "b", "X")     -- "aaXXcc"
local s = strings.replace("aabbcc", "b", "X", 1)  -- "aaXbcc"
```

---

### `strings.upper(str)`

Convert to uppercase.

```lua
local s = strings.upper("hello")  -- "HELLO"
```

---

### `strings.lower(str)`

Convert to lowercase.

```lua
local s = strings.lower("HELLO")  -- "hello"
```

---

### `strings.format(fmt, ...)`

Format a string using Go `fmt.Sprintf` syntax.

```lua
local s = strings.format("%s has %d items", "list", 5)
-- "list has 5 items"
```
