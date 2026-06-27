# `regex` -- Regular Expressions

```lua
local re = require("regex")
```

Regular expression support using Go's RE2 syntax. More powerful than Lua's built-in pattern matching -- supports alternation (`|`), non-greedy quantifiers, named captures, etc.

## Functions

### `re.match(pattern, str)`

Returns `true` if `pattern` matches anywhere in `str`.

```lua
if re.match("^\\d+$", "12345") then ... end
```

Returns: `bool`

---

### `re.find(pattern, str)`

Returns the first match and any capture groups, or `nil` if no match.

```lua
local m = re.find("(\\w+)@(\\w+)", "user@example.com")
if m then
    print(m[1])  -- "user@example"  (full match)
    print(m[2])  -- "user"          (capture 1)
    print(m[3])  -- "example"       (capture 2)
end
```

Returns: `table|nil` -- array where `[1]` is the full match, `[2]`... are capture groups

---

### `re.find_all(pattern, str [, limit])`

Returns all non-overlapping matches. Pass `-1` (or omit) for no limit.

```lua
local matches = re.find_all("\\d+", "a1 b22 c333")
-- matches = {"1", "22", "333"}
```

Returns: `table`

---

### `re.replace(pattern, str, repl)`

Replace all matches of `pattern` in `str` with `repl`.

```lua
local result = re.replace("\\s+", "hello   world", " ")
-- "hello world"
```

Returns: `string`

---

### `re.split(pattern, str [, limit])`

Split `str` by the regex pattern.

```lua
local parts = re.split("\\s+", "one  two   three")
-- {"one", "two", "three"}
```

Returns: `table`

---

### `re.is_valid(pattern)`

Returns `true` if the pattern compiles without error.

```lua
if not re.is_valid(user_pattern) then
    log.error("invalid regex")
end
```

Returns: `bool`

---

## Notes

- Uses Go RE2 syntax. Lookaheads and backreferences are not supported.
- Double-backslash required in Lua strings: `"\\d+"` matches digits.
