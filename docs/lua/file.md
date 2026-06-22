# `file` -- File Read/Write

```lua
local file = require("file")
-- or:
local file = require("riverdeck.file")
```

Provides file system operations. All paths are restricted to the Riverdeck config directory for security. Attempting to access paths outside the config directory returns an `"access denied"` error.

## Functions

### `file.read(path)`

Read the entire contents of a file.

```lua
local content, err = file.read("/path/to/file.txt")
if err then error(err) end
```

Returns: `string|nil, err|nil`

---

### `file.write(path, content)`

Write content to a file, replacing any existing content.

```lua
local ok, err = file.write("/path/to/file.txt", "hello\n")
```

Returns: `bool, err|nil`

---

### `file.append(path, content)`

Append content to a file. Creates the file if it does not exist.

```lua
local ok, err = file.append("/path/to/log.txt", "line\n")
```

Returns: `bool, err|nil`

---

### `file.exists(path)`

Returns `true` if the path exists (file or directory).

```lua
if file.exists("/path/to/file.txt") then ... end
```

Returns: `bool`

---

### `file.is_dir(path)`

Returns `true` if the path is a directory.

```lua
if file.is_dir("/path/to/dir") then ... end
```

Returns: `bool`

---

### `file.size(path)`

Returns the file size in bytes, or `-1` on error.

```lua
local size = file.size("/path/to/file.txt")
```

Returns: `number`

---

### `file.mkdir(path)`

Create a directory and all parent directories.

```lua
local ok, err = file.mkdir("/path/to/new/dir")
```

Returns: `bool, err|nil`

---

### `file.list(path)`

List the contents of a directory. Returns an array of entry tables.

```lua
local entries, err = file.list("/path/to/dir")
for _, entry in ipairs(entries) do
    print(entry.name, entry.is_dir, entry.size)
end
```

Each entry table has:
- `name` (string) -- filename
- `is_dir` (bool) -- true if directory
- `size` (number) -- file size in bytes

Returns: `table|nil, err|nil`

---

### `file.remove(path)`

Delete a file. Does not remove directories.

```lua
local ok, err = file.remove("/path/to/file.txt")
```

Returns: `bool, err|nil`

---

## Notes

- Use `CONFIG_DIR` (a global injected into every script) to build paths relative to the config directory.
- For package-scoped storage, use [`pkg_data`](pkg_data.md) instead.

```lua
local config_file = CONFIG_DIR .. "/apps/myapp.json"
local content, err = file.read(config_file)
```
