package modules

// pkg_data.go - package-scoped filesystem and JSON storage, available ONLY to
// scripts that are part of an installed package (daemon.lua, lib/*.lua, etc.).
// Regular button scripts and .directory.lua files do NOT receive this module.
//
// The module is scoped to a single directory per package:
//
//	<configDir>/.packages/<vendor.pkg>/data/
//
// All paths passed to pkg_data functions are relative to that directory.
// Absolute paths and path traversal (../) are silently rejected.
//
// Lua API:
//
//	local d = require('pkg_data')
//
//	-- File I/O
//	local text, err = d.read("config.txt")
//	local ok,  err = d.write("config.txt",  "hello\n")
//	local ok,  err = d.append("log.txt",    "line\n")
//
//	-- JSON helpers
//	local tbl, err = d.json_read("auth.json")         -- decode file -> table
//	local ok,  err = d.json_write("auth.json", tbl)   -- encode table -> file
//
//	-- Queries
//	local exists     = d.exists("auth.json")           -- bool
//	local is_dir     = d.is_dir("subdir")              -- bool
//	local files, err = d.list()                        -- {name, ...}
//	local files, err = d.list("subdir")
//	local ok,  err   = d.mkdir("subdir")
//	local ok,  err   = d.remove("old.txt")
//
//	-- Meta
//	local path = d.path()     -- absolute path to the data directory
//	local path = d.path("f")  -- absolute path to data/f

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/merith-tk/riverdeck/pkg/lualib"
	lua "github.com/yuin/gopher-lua"
)

// PackageDataModule provides sandboxed file and JSON I/O for a single package.
type PackageDataModule struct {
	// dataDir is the absolute path to the package's data directory.
	// All operations are restricted to this directory tree.
	dataDir string
}

// NewPackageDataModule creates a module rooted at dataDir.
// The directory is created if it does not yet exist.
func NewPackageDataModule(dataDir string) (*PackageDataModule, error) {
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return nil, fmt.Errorf("pkg_data: cannot create data dir %s: %w", dataDir, err)
	}
	return &PackageDataModule{dataDir: dataDir}, nil
}

// Loader is the gopher-lua module loader. Register with:
//
//	L.PreloadModule("pkg_data", mod.Loader)
func (m *PackageDataModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"read":       m.read,
		"write":      m.write,
		"append":     m.appendFile,
		"json_read":  m.jsonRead,
		"json_write": m.jsonWrite,
		"exists":     m.exists,
		"is_dir":     m.isDir,
		"list":       m.list,
		"mkdir":      m.mkdir,
		"remove":     m.remove,
		"path":       m.path,
	})
	L.Push(mod)
	return 1
}

// resolve turns a relative Lua path into an absolute OS path, rejecting any
// attempt to escape the data directory.
func (m *PackageDataModule) resolve(rel string) (string, error) {
	if rel == "" {
		return m.dataDir, nil
	}
	// Reject absolute paths.
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed; use relative paths within pkg_data")
	}
	abs := filepath.Clean(filepath.Join(m.dataDir, rel))
	// Guard against ../../../etc traversal.
	if !strings.HasPrefix(abs, m.dataDir) {
		return "", fmt.Errorf("path escapes package data directory")
	}
	return abs, nil
}

// -- File I/O ------------------------------------------------------------------

// read(file) -> string|nil, err|nil
func (m *PackageDataModule) read(L *lua.LState) int {
	rel := L.CheckString(1)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}

// write(file, content) -> ok, err|nil
func (m *PackageDataModule) write(L *lua.LState) int {
	rel := L.CheckString(1)
	content := L.CheckString(2)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	// Ensure parent directory exists.
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := os.WriteFile(abs, []byte(content), 0644); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// append(file, content) -> ok, err|nil
func (m *PackageDataModule) appendFile(L *lua.LState) int {
	rel := L.CheckString(1)
	content := L.CheckString(2)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	f, err := os.OpenFile(abs, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	defer f.Close()
	if _, err := f.WriteString(content); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// -- JSON helpers --------------------------------------------------------------

// json_read(file) -> table|nil, err|nil
func (m *PackageDataModule) jsonRead(L *lua.LState) int {
	rel := L.CheckString(1)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	var raw interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lualib.GoToLua(L, raw))
	L.Push(lua.LNil)
	return 2
}

// json_write(file, table) -> ok, err|nil
func (m *PackageDataModule) jsonWrite(L *lua.LState) int {
	rel := L.CheckString(1)
	val := L.CheckAny(2)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	raw := lualib.LuaToGo(val)
	data, err := json.MarshalIndent(raw, "", "  ")
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0755); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := os.WriteFile(abs, data, 0644); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// -- Queries -------------------------------------------------------------------

// exists(file) -> bool
func (m *PackageDataModule) exists(L *lua.LState) int {
	rel := L.CheckString(1)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		return 1
	}
	_, err = os.Stat(abs)
	L.Push(lua.LBool(err == nil))
	return 1
}

// is_dir(path) -> bool
func (m *PackageDataModule) isDir(L *lua.LState) int {
	rel := L.CheckString(1)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		return 1
	}
	info, err := os.Stat(abs)
	L.Push(lua.LBool(err == nil && info.IsDir()))
	return 1
}

// list([subdir]) -> table|nil, err|nil    -- returns array of entry names
func (m *PackageDataModule) list(L *lua.LState) int {
	rel := L.OptString(1, "")
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	entries, err := os.ReadDir(abs)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	for i, e := range entries {
		tbl.RawSetInt(i+1, lua.LString(e.Name()))
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

// mkdir(subdir) -> ok, err|nil
func (m *PackageDataModule) mkdir(L *lua.LState) int {
	rel := L.CheckString(1)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := os.MkdirAll(abs, 0755); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// remove(file) -> ok, err|nil
func (m *PackageDataModule) remove(L *lua.LState) int {
	rel := L.CheckString(1)
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	if err := os.Remove(abs); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// path([rel]) -> string
func (m *PackageDataModule) path(L *lua.LState) int {
	rel := L.OptString(1, "")
	if rel == "" {
		L.Push(lua.LString(m.dataDir))
		return 1
	}
	abs, err := m.resolve(rel)
	if err != nil {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(abs))
	return 1
}

// Lua↔Go conversion helpers are provided by the lualib package
// (lualib.GoToLua, lualib.LuaToGo) to avoid duplication.
