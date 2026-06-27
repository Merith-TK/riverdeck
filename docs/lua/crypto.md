# `crypto` -- Hashing

```lua
local crypto = require("crypto")
```

Basic cryptographic hashing. Intended for cache keys, API signatures, and data integrity checks -- not for password hashing or key derivation.

## Functions

### `crypto.md5(str)`

Returns the hex-encoded MD5 hash.

```lua
local hash = crypto.md5("hello")
-- "5d41402abc4b2a76b9719d911017c592"
```

Returns: `string`

---

### `crypto.sha1(str)`

Returns the hex-encoded SHA-1 hash.

```lua
local hash = crypto.sha1("hello")
-- "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
```

Returns: `string`

---

### `crypto.sha256(str)`

Returns the hex-encoded SHA-256 hash.

```lua
local hash = crypto.sha256("hello")
-- "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
```

Returns: `string`
