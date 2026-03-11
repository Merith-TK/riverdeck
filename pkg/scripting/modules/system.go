// Package modules provides Lua module loaders for the Riverdeck scripting system.
//
// Each module is registered as a preloaded Lua module and accessed inside
// scripts with require(). Modules are bound to runtime context (device,
// config directory, refresh callback) at script load time.
//
// Available modules:
//
//	shell      - execute shell commands, open files/URLs, launch terminals
//	http       - HTTP GET / POST / custom requests
//	system     - OS detection, environment, sleep (yield), refresh
//	streamdeck - direct hardware control (brightness, key colour, layout)
//	file       - read/write files within the config directory
//
// The lualib package provides additional pure-Go stdlib replacements:
//
//	utils      - table utilities (deepcopy, contains, merge)
//	strings    - string utilities (split, trim, case conversion, ...)
//	json       - JSON encode / decode
//	time       - Unix timestamps, date decomposition, blocking sleep
//	log        - levelled logging (info, warn, error, debug)
package modules

import (
	"os"
	"runtime"

	lua "github.com/yuin/gopher-lua"
)

// SystemModule provides OS/system utilities to Lua scripts.
type SystemModule struct {
	onRefresh func() // called when script requests a display refresh
}

// NewSystemModule creates a new system module.
// onRefresh is called when a script invokes system.refresh(); pass nil to disable.
func NewSystemModule(onRefresh func()) *SystemModule {
	return &SystemModule{onRefresh: onRefresh}
}

// Loader returns the Lua module loader function.
func (m *SystemModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"os":       m.systemOS,
		"env":      m.systemEnv,
		"sleep":    m.systemSleep,
		"hostname": m.systemHostname,
		"refresh":  m.systemRefresh,
	})
	L.Push(mod)
	return 1
}

// systemOS returns the current operating system name (e.g. "linux", "windows").
// Lua: system.os() -> string
func (m *SystemModule) systemOS(L *lua.LState) int {
	L.Push(lua.LString(runtime.GOOS))
	return 1
}

// systemEnv returns an environment variable value, or nil if unset.
// Lua: system.env(key) -> string|nil
func (m *SystemModule) systemEnv(L *lua.LState) int {
	key := L.CheckString(1)
	value := os.Getenv(key)
	if value == "" {
		L.Push(lua.LNil)
	} else {
		L.Push(lua.LString(value))
	}
	return 1
}

// systemSleep sleeps for ms milliseconds.
// In background scripts this yields the coroutine so other work can proceed.
// In trigger/passive callbacks it blocks briefly (capped at 500ms).
// Lua: system.sleep(ms)
func (m *SystemModule) systemSleep(L *lua.LState) int {
	ms := L.CheckInt(1)
	// Yield to Go scheduler; the background loop in runner.go reads the sleep
	// duration from the coroutine's yield value and waits via time.After.
	return L.Yield(lua.LNumber(ms))
}

// systemHostname returns the machine hostname.
// Lua: system.hostname() -> string|nil
func (m *SystemModule) systemHostname(L *lua.LState) int {
	name, err := os.Hostname()
	if err != nil {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(name))
	return 1
}

// systemRefresh requests an immediate display refresh from the runner.
// Lua: system.refresh()
func (m *SystemModule) systemRefresh(L *lua.LState) int {
	if m.onRefresh != nil {
		m.onRefresh()
	}
	return 0
}
