// Package lualib provides Go-native implementations of Lua standard library
// modules for use with the NOMAD scripting system. Registering these as
// preloaded modules is faster than loading .lua files from disk at runtime.
package lualib

import lua "github.com/yuin/gopher-lua"

// RegisterUtils preloads the "utils" module into the given Lua state.
// Lua scripts access it via: local utils = require("utils")
func RegisterUtils(L *lua.LState) {
	L.PreloadModule("utils", utilsLoader)
}

func utilsLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"deepcopy": utilsDeepCopy,
		"contains": utilsContains,
		"size":     utilsSize,
		"merge":    utilsMerge,
	})
	L.Push(mod)
	return 1
}

// utilsDeepCopy recursively copies a Lua table.
// Lua: utils.deepcopy(t) -> copy
func utilsDeepCopy(L *lua.LState) int {
	val := L.CheckAny(1)
	L.Push(deepCopy(L, val))
	return 1
}

func deepCopy(L *lua.LState, val lua.LValue) lua.LValue {
	tbl, ok := val.(*lua.LTable)
	if !ok {
		return val // primitives are value types - no copy needed
	}
	newTbl := L.NewTable()
	tbl.ForEach(func(k, v lua.LValue) {
		newTbl.RawSet(deepCopy(L, k), deepCopy(L, v))
	})
	// Copy metatable if present
	if mt := L.GetMetatable(tbl); mt != lua.LNil {
		L.SetMetatable(newTbl, mt)
	}
	return newTbl
}

// utilsContains checks whether a table contains a given value.
// Lua: utils.contains(t, value) -> bool
func utilsContains(L *lua.LState) int {
	tbl := L.CheckTable(1)
	target := L.CheckAny(2)
	found := false
	tbl.ForEach(func(_, v lua.LValue) {
		if v == target {
			found = true
		}
	})
	L.Push(lua.LBool(found))
	return 1
}

// utilsSize returns the number of entries in a table (including non-sequence).
// Lua: utils.size(t) -> number
func utilsSize(L *lua.LState) int {
	tbl := L.CheckTable(1)
	count := 0
	tbl.ForEach(func(_, _ lua.LValue) { count++ })
	L.Push(lua.LNumber(count))
	return 1
}

// utilsMerge copies all keys from t2 into a deep copy of t1 (t2 wins on conflict).
// Lua: utils.merge(t1, t2) -> merged
func utilsMerge(L *lua.LState) int {
	t1 := L.CheckTable(1)
	t2 := L.CheckTable(2)
	result, _ := deepCopy(L, t1).(*lua.LTable)
	t2.ForEach(func(k, v lua.LValue) {
		result.RawSet(k, v)
	})
	L.Push(result)
	return 1
}
