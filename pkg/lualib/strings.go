// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"strings"
	"unicode"

	lua "github.com/yuin/gopher-lua"
)

// RegisterStrings preloads the "strings" module into the given Lua state.
// Lua scripts access it via: local strings = require("strings")
func RegisterStrings(L *lua.LState) {
	L.PreloadModule("strings", stringsLoader)
}

func stringsLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"split":      stringsSplit,
		"trim":       stringsTrim,
		"startswith": stringsStartsWith,
		"endswith":   stringsEndsWith,
		"capitalize": stringsCapitalize,
		"titlecase":  stringsTitleCase,
		"contains":   stringsContains,
		"replace":    stringsReplace,
		"upper":      stringsUpper,
		"lower":      stringsLower,
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

// stringsTrim removes leading and trailing whitespace.
// Lua: strings.trim(str) -> str
func stringsTrim(L *lua.LState) int {
	L.Push(lua.LString(strings.TrimSpace(L.CheckString(1))))
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

// stringsCapitalize uppercases the first character of str.
// Lua: strings.capitalize(str) -> str
func stringsCapitalize(L *lua.LState) int {
	s := L.CheckString(1)
	if s == "" {
		L.Push(lua.LString(""))
		return 1
	}
	runes := []rune(s)
	runes[0] = unicode.ToUpper(runes[0])
	L.Push(lua.LString(string(runes)))
	return 1
}

// stringsTitleCase title-cases a string (first letter of each word uppercased).
// Lua: strings.titlecase(str) -> str
func stringsTitleCase(L *lua.LState) int {
	L.Push(lua.LString(strings.Title(L.CheckString(1)))) //nolint:staticcheck
	return 1
}

// stringsContains returns true if substr is found in str.
// Lua: strings.contains(str, substr) -> bool
func stringsContains(L *lua.LState) int {
	L.Push(lua.LBool(strings.Contains(L.CheckString(1), L.CheckString(2))))
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
