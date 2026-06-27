# `base64` -- Base64 Encoding/Decoding

```lua
local b64 = require("base64")
```

Encode and decode base64 strings. Supports both standard (padded) and URL-safe (unpadded) variants.

## Functions

### `b64.encode(str)`

Encode a string to standard base64.

```lua
local encoded = b64.encode("hello")  -- "aGVsbG8="
```

Returns: `string`

---

### `b64.decode(str)`

Decode a standard base64 string.

```lua
local decoded, err = b64.decode("aGVsbG8=")  -- "hello"
```

Returns: `string|nil, err|nil`

---

### `b64.url_encode(str)`

Encode to URL-safe base64 (no padding characters).

```lua
local encoded = b64.url_encode("hello")  -- "aGVsbG8"
```

Returns: `string`

---

### `b64.url_decode(str)`

Decode a URL-safe base64 string (no padding).

```lua
local decoded, err = b64.url_decode("aGVsbG8")  -- "hello"
```

Returns: `string|nil, err|nil`

---

## Example: Basic Auth Header

```lua
local b64 = require("base64")
local http = require("http")

local creds = b64.encode("user:password")
local body, status = http.request("GET", "https://example.com/api",
    { ["Authorization"] = "Basic " .. creds }
)
```
