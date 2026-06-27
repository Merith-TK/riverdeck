# `json` -- JSON Encode/Decode

```lua
local json = require("json")
```

Encode Lua values to JSON strings and decode JSON strings back to Lua values.

## Functions

### `json.encode(value)`

Encode a Lua value to a JSON string.

```lua
local str, err = json.encode({ name = "Alice", age = 30 })
-- str = '{"age":30,"name":"Alice"}'

local str, err = json.encode({ 1, 2, 3 })
-- str = '[1,2,3]'
```

Tables with consecutive integer keys starting at 1 are encoded as JSON arrays. All other tables are encoded as JSON objects.

Returns: `string|nil, err|nil`

---

### `json.decode(str)`

Decode a JSON string into a Lua value.

```lua
local data, err = json.decode('{"key": "value", "count": 42}')
if err then error(err) end
print(data.key)    -- "value"
print(data.count)  -- 42
```

Returns: `table|string|number|bool|nil, err|nil`

---

## Type Mapping

| JSON | Lua |
|------|-----|
| `object` | table (string keys) |
| `array` | table (integer keys 1..n) |
| `string` | string |
| `number` | number |
| `true`/`false` | bool |
| `null` | nil |
