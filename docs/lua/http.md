# `http` -- HTTP Requests

```lua
local http = require("http")
-- or:
local http = require("riverdeck.http")
```

Provides HTTP client functionality. The client has a 15 second default timeout and reuses connections via a connection pool.

## Functions

### `http.get(url)`

Perform a GET request.

```lua
local body, status = http.get("https://example.com/api/data")
if not body then
    -- status contains the error message in this case
    error(status)
end
```

Returns: `string|nil, number|string` -- (body, status_code) on success; (nil, error_message) on failure

---

### `http.post(url, content_type, body)`

Perform a POST request.

```lua
local body, status = http.post(
    "https://example.com/api",
    "application/json",
    '{"key": "value"}'
)
```

Returns: `string|nil, number|string`

---

### `http.request(method, url [, headers [, body [, timeout_ms]]])`

Perform a custom HTTP request with full control over method, headers, body, and timeout.

```lua
local body, status = http.request(
    "PUT",
    "https://example.com/api/resource",
    { ["Authorization"] = "Bearer token123", ["Content-Type"] = "application/json" },
    '{"value": 42}',
    5000  -- optional timeout in milliseconds (0 = use default 15s)
)
```

| Parameter | Type | Description |
|-----------|------|-------------|
| `method` | string | HTTP method (GET, POST, PUT, DELETE, PATCH, etc.) |
| `url` | string | Target URL |
| `headers` | table\|nil | Key-value pairs added as request headers |
| `body` | string | Request body (empty string for no body) |
| `timeout_ms` | number | Per-request timeout in milliseconds (0 = default) |

Returns: `string|nil, number|string`

---

## Example: Polling a JSON API

```lua
local http = require("http")
local json = require("json")

return {
    label = function()
        local body, status = http.get("https://api.example.com/status")
        if not body then return "ERR" end
        local data, err = json.decode(body)
        if err then return "ERR" end
        return data.state or "?"
    end
}
```
