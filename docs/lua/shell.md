# `shell` -- Shell Commands

```lua
local shell = require("shell")
-- or:
local shell = require("riverdeck.shell")
```

Provides shell command execution and OS-level operations. Commands run via the system shell (`sh -c` on Linux/macOS, `cmd /c` on Windows).

## Functions

### `shell.exec(command)`

Run a shell command and wait for it to finish.

```lua
local stdout, stderr, exit_code = shell.exec("echo hello")
```

Returns: `string, string, number` -- (stdout, stderr, exit_code)

On error starting the process: returns `nil, error_message, -1`

---

### `shell.exec_async(command)`

Start a shell command and return immediately without waiting.

```lua
local ok, err = shell.exec_async("notify-send 'Button pressed'")
```

Returns: `bool, err|nil`

---

### `shell.open(target)`

Open a file, URL, or directory using the OS default handler (`xdg-open` / `open` / `start`).

```lua
shell.open("https://example.com")
shell.open("/home/user/documents")
shell.open("/home/user/file.pdf")
```

Returns: `bool, err|nil`

---

### `shell.terminal(command)`

Open a new terminal window and run a command in it.

On Linux, tries terminal emulators in order: `x-terminal-emulator`, `gnome-terminal`, `konsole`, `xfce4-terminal`, `xterm`.

```lua
local ok, err = shell.terminal("htop")
```

Returns: `bool, err|nil`

---

## Example: Launch App on Press

```lua
local shell = require("shell")

return {
    label = function() return "Spotify" end,
    trigger = function()
        shell.exec_async("spotify")
    end
}
```

## Example: Show Output on Button

```lua
local shell = require("shell")

return {
    label = function()
        local out, _, code = shell.exec("uptime -p")
        if code ~= 0 then return "ERR" end
        return out:match("up (.-)%s*$") or "?"
    end
}
```
