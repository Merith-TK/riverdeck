// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"path/filepath"

	lua "github.com/yuin/gopher-lua"
)

// RegisterPath preloads the "path" module into the given Lua state.
// Lua scripts access it via: local path = require("path")
//
// Wraps Go's filepath package so Lua scripts can construct and decompose
// file paths without platform-specific string hacking.
func RegisterPath(L *lua.LState) {
	L.PreloadModule("path", pathLoader)
}

func pathLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"join":      pathJoin,
		"dir":       pathDir,
		"base":      pathBase,
		"ext":       pathExt,
		"clean":     pathClean,
		"is_abs":    pathIsAbs,
		"rel":       pathRel,
		"separator": pathSeparator,
	})
	L.Push(mod)
	return 1
}

// pathJoin joins path elements into a single path.
// Lua: path.join("a", "b", "c") -> "a/b/c"
func pathJoin(L *lua.LState) int {
	n := L.GetTop()
	parts := make([]string, n)
	for i := 1; i <= n; i++ {
		parts[i-1] = L.CheckString(i)
	}
	L.Push(lua.LString(filepath.Join(parts...)))
	return 1
}

// pathDir returns all but the last element of path (the parent directory).
// Lua: path.dir("/a/b/c") -> "/a/b"
func pathDir(L *lua.LState) int {
	L.Push(lua.LString(filepath.Dir(L.CheckString(1))))
	return 1
}

// pathBase returns the last element of path (the filename).
// Lua: path.base("/a/b/c.txt") -> "c.txt"
func pathBase(L *lua.LState) int {
	L.Push(lua.LString(filepath.Base(L.CheckString(1))))
	return 1
}

// pathExt returns the file extension including the dot.
// Lua: path.ext("file.txt") -> ".txt"
func pathExt(L *lua.LState) int {
	L.Push(lua.LString(filepath.Ext(L.CheckString(1))))
	return 1
}

// pathClean returns the cleaned/normalised form of the path.
// Lua: path.clean("a//b/../c") -> "a/c"
func pathClean(L *lua.LState) int {
	L.Push(lua.LString(filepath.Clean(L.CheckString(1))))
	return 1
}

// pathIsAbs reports whether the path is absolute.
// Lua: path.is_abs("/foo") -> true
func pathIsAbs(L *lua.LState) int {
	L.Push(lua.LBool(filepath.IsAbs(L.CheckString(1))))
	return 1
}

// pathRel returns a relative path from base to target.
// Lua: path.rel("/a/b", "/a/b/c/d") -> "c/d", nil
func pathRel(L *lua.LState) int {
	basePath := L.CheckString(1)
	targPath := L.CheckString(2)
	rel, err := filepath.Rel(basePath, targPath)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(rel))
	L.Push(lua.LNil)
	return 2
}

// pathSeparator returns the OS-specific path separator as a string.
// Lua: path.separator() -> "/" or "\\"
func pathSeparator(L *lua.LState) int {
	L.Push(lua.LString(string(filepath.Separator)))
	return 1
}
