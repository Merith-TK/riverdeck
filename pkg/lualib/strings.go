// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"fmt"
	"strings"

	lua "github.com/yuin/gopher-lua"
)

// RegisterStrings preloads the "strings" module into the given Lua state.
// Lua scripts access it via: local strings = require("strings")
func RegisterStrings(L *lua.LState) {
	L.PreloadModule("strings", stringsLoader)
}

func stringsLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"split":       stringsSplit,
		"join":        stringsJoin,
		"trim":        stringsTrim,
		"trim_prefix": stringsTrimPrefix,
		"trim_suffix": stringsTrimSuffix,
		"startswith":  stringsStartsWith,
		"endswith":    stringsEndsWith,
		"contains":    stringsContains,
		"index":       stringsIndex,
		"count":       stringsCount,
		"replace":     stringsReplace,
		"upper":       stringsUpper,
		"lower":       stringsLower,
		"format":      stringsFormat,
	})
	L.Push(mod)
	return 1
}

// stringsSplit splits str by sep and returns a Lua array table.
// Lua: strings.split(str, sep) -> table
func stringsSplit(L *lua.LState) int {
	str := L.CheckString(1)
	sep := L.CheckString(2)
	parts := strings.Split(str, sep)
	tbl := L.NewTable()
	for i, p := range parts {
		tbl.RawSetInt(i+1, lua.LString(p))
	}
	L.Push(tbl)
	return 1
}

// stringsJoin joins a Lua table of strings with a separator.
// Lua: strings.join(tbl, sep) -> str
func stringsJoin(L *lua.LState) int {
	tbl := L.CheckTable(1)
	sep := L.CheckString(2)
	parts := make([]string, 0, tbl.Len())
	for i := 1; i <= tbl.Len(); i++ {
		parts = append(parts, tbl.RawGetInt(i).String())
	}
	L.Push(lua.LString(strings.Join(parts, sep)))
	return 1
}

// stringsTrim removes leading and trailing whitespace.
// Lua: strings.trim(str) -> str
func stringsTrim(L *lua.LState) int {
	L.Push(lua.LString(strings.TrimSpace(L.CheckString(1))))
	return 1
}

// stringsTrimPrefix removes a leading prefix string.
// Lua: strings.trim_prefix(str, prefix) -> str
func stringsTrimPrefix(L *lua.LState) int {
	L.Push(lua.LString(strings.TrimPrefix(L.CheckString(1), L.CheckString(2))))
	return 1
}

// stringsTrimSuffix removes a trailing suffix string.
// Lua: strings.trim_suffix(str, suffix) -> str
func stringsTrimSuffix(L *lua.LState) int {
	L.Push(lua.LString(strings.TrimSuffix(L.CheckString(1), L.CheckString(2))))
	return 1
}

// stringsStartsWith returns true if str starts with prefix.
// Lua: strings.startswith(str, prefix) -> bool
func stringsStartsWith(L *lua.LState) int {
	L.Push(lua.LBool(strings.HasPrefix(L.CheckString(1), L.CheckString(2))))
	return 1
}

// stringsEndsWith returns true if str ends with suffix.
// Lua: strings.endswith(str, suffix) -> bool
func stringsEndsWith(L *lua.LState) int {
	L.Push(lua.LBool(strings.HasSuffix(L.CheckString(1), L.CheckString(2))))
	return 1
}

// stringsContains returns true if substr is found in str.
// Lua: strings.contains(str, substr) -> bool
func stringsContains(L *lua.LState) int {
	L.Push(lua.LBool(strings.Contains(L.CheckString(1), L.CheckString(2))))
	return 1
}

// stringsIndex returns the 1-based position of substr in str, or 0 if not found.
// Lua: strings.index(str, substr) -> number
func stringsIndex(L *lua.LState) int {
	idx := strings.Index(L.CheckString(1), L.CheckString(2))
	if idx < 0 {
		L.Push(lua.LNumber(0))
	} else {
		L.Push(lua.LNumber(idx + 1)) // 1-based
	}
	return 1
}

// stringsCount counts non-overlapping occurrences of substr in str.
// Lua: strings.count(str, substr) -> number
func stringsCount(L *lua.LState) int {
	L.Push(lua.LNumber(strings.Count(L.CheckString(1), L.CheckString(2))))
	return 1
}

// stringsReplace replaces occurrences of old with new in str.
// Pass -1 as count to replace all.
// Lua: strings.replace(str, old, new [, count]) -> str
func stringsReplace(L *lua.LState) int {
	str := L.CheckString(1)
	old := L.CheckString(2)
	newStr := L.CheckString(3)
	count := L.OptInt(4, -1)
	L.Push(lua.LString(strings.Replace(str, old, newStr, count)))
	return 1
}

// stringsUpper converts str to uppercase.
// Lua: strings.upper(str) -> str
func stringsUpper(L *lua.LState) int {
	L.Push(lua.LString(strings.ToUpper(L.CheckString(1))))
	return 1
}

// stringsLower converts str to lowercase.
// Lua: strings.lower(str) -> str
func stringsLower(L *lua.LState) int {
	L.Push(lua.LString(strings.ToLower(L.CheckString(1))))
	return 1
}

// stringsFormat formats a string (Go fmt.Sprintf syntax).
// Lua: strings.format("%s has %d items", name, count) -> str
func stringsFormat(L *lua.LState) int {
	fmtStr := L.CheckString(1)
	args := make([]interface{}, L.GetTop()-1)
	for i := 2; i <= L.GetTop(); i++ {
		switch v := L.Get(i).(type) {
		case lua.LBool:
			args[i-2] = bool(v)
		case lua.LNumber:
			args[i-2] = float64(v)
		case lua.LString:
			args[i-2] = string(v)
		default:
			args[i-2] = v.String()
		}
	}
	L.Push(lua.LString(fmt.Sprintf(fmtStr, args...)))
	return 1
}
