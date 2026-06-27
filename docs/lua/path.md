# `path` -- File Path Utilities

```lua
local path = require("path")
```

Cross-platform file path construction and decomposition, backed by Go's `filepath` package.

## Functions

### `path.join(...)`

Join path elements into a single path.

```lua
local p = path.join("a", "b", "c")  -- "a/b/c"
local p = path.join(CONFIG_DIR, "apps", "myapp.json")
```

Returns: `string`

---

### `path.dir(p)`

Return all but the last element (the parent directory).

```lua
local dir = path.dir("/a/b/c.txt")  -- "/a/b"
```

Returns: `string`

---

### `path.base(p)`

Return the last element (the filename or directory name).

```lua
local name = path.base("/a/b/c.txt")  -- "c.txt"
```

Returns: `string`

---

### `path.ext(p)`

Return the file extension including the dot.

```lua
local ext = path.ext("file.txt")  -- ".txt"
local ext = path.ext("noext")     -- ""
```

Returns: `string`

---

### `path.clean(p)`

Return the cleaned/normalised form of a path.

```lua
local p = path.clean("a//b/../c")  -- "a/c"
```

Returns: `string`

---

### `path.is_abs(p)`

Returns `true` if the path is absolute.

```lua
if path.is_abs(p) then ... end
```

Returns: `bool`

---

### `path.rel(base, target)`

Return a relative path from `base` to `target`.

```lua
local rel, err = path.rel("/a/b", "/a/b/c/d")
-- rel = "c/d"
```

Returns: `string|nil, err|nil`

---

### `path.separator()`

Return the OS path separator as a string (`"/"` or `"\\"`).

```lua
local sep = path.separator()
```

Returns: `string`
