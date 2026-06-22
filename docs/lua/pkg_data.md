# `pkg_data` -- Package-Scoped File Storage

```lua
local d = require("pkg_data")
-- or:
local d = require("riverdeck.pkg_data")
```

Sandboxed file and JSON storage scoped to a single installed package. All paths are relative to the package's data directory:

```
~/.config/riverdeck/.config/packages/<vendor.pkg>/data/
```

Absolute paths and path traversal (`../`) are rejected. Only scripts that are part of an installed package (daemon, library files, etc.) have access to this module. Regular button scripts do not.

## File I/O

### `d.read(file)`

Read a file relative to the data directory.

```lua
local text, err = d.read("config.txt")
```

Returns: `string|nil, err|nil`

---

### `d.write(file, content)`

Write content to a file. Creates parent directories automatically.

```lua
local ok, err = d.write("config.txt", "hello\n")
```

Returns: `bool, err|nil`

---

### `d.append(file, content)`

Append content to a file. Creates the file if it does not exist.

```lua
local ok, err = d.append("log.txt", "entry\n")
```

Returns: `bool, err|nil`

---

## JSON Helpers

### `d.json_read(file)`

Read a JSON file and decode it to a Lua table.

```lua
local data, err = d.json_read("auth.json")
if data then
    print(data.token)
end
```

Returns: `table|nil, err|nil`

---

### `d.json_write(file, table)`

Encode a Lua table as JSON and write it to a file.

```lua
local ok, err = d.json_write("auth.json", { token = "abc123", expires = 1234567890 })
```

Returns: `bool, err|nil`

---

## Queries

### `d.exists(file)`

```lua
if d.exists("auth.json") then ... end
```

Returns: `bool`

---

### `d.is_dir(path)`

```lua
if d.is_dir("subdir") then ... end
```

Returns: `bool`

---

### `d.list([subdir])`

List file names in the data directory (or a subdirectory).

```lua
local files, err = d.list()
local files, err = d.list("subdir")
```

Returns: `table|nil, err|nil` (array of filename strings)

---

### `d.mkdir(subdir)`

Create a subdirectory.

```lua
local ok, err = d.mkdir("cache")
```

Returns: `bool, err|nil`

---

### `d.remove(file)`

Delete a file.

```lua
local ok, err = d.remove("old-cache.json")
```

Returns: `bool, err|nil`

---

## Path Resolution

### `d.path([rel])`

Get the absolute path to the data directory, or to a file within it.

```lua
local dir = d.path()         -- absolute path to data/
local file = d.path("f.txt") -- absolute path to data/f.txt
```

Returns: `string|nil`
