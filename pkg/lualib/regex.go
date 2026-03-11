// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"regexp"

	lua "github.com/yuin/gopher-lua"
)

// RegisterRegex preloads the "regex" module into the given Lua state.
// Lua scripts access it via: local re = require("regex")
//
// Exposes Go's regexp (RE2 syntax) to Lua, which is significantly more
// powerful than Lua's built-in pattern matching (e.g. alternation, non-greedy
// quantifiers, named capture groups).
func RegisterRegex(L *lua.LState) {
	L.PreloadModule("regex", regexLoader)
}

func regexLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"match":    regexMatch,
		"find":     regexFind,
		"find_all": regexFindAll,
		"replace":  regexReplace,
		"split":    regexSplit,
		"is_valid": regexIsValid,
	})
	L.Push(mod)
	return 1
}

// regexMatch tests whether pattern matches the string.
// Lua: regex.match(pattern, str) -> bool
func regexMatch(L *lua.LState) int {
	pattern := L.CheckString(1)
	str := L.CheckString(2)
	re, err := regexp.Compile(pattern)
	if err != nil {
		L.ArgError(1, "invalid regex: "+err.Error())
		return 0
	}
	L.Push(lua.LBool(re.MatchString(str)))
	return 1
}

// regexFind returns the first match (and sub-matches) or nil.
// Lua: regex.find(pattern, str) -> table|nil
// The returned table has the full match at [1], followed by capture groups.
func regexFind(L *lua.LState) int {
	pattern := L.CheckString(1)
	str := L.CheckString(2)
	re, err := regexp.Compile(pattern)
	if err != nil {
		L.ArgError(1, "invalid regex: "+err.Error())
		return 0
	}
	matches := re.FindStringSubmatch(str)
	if matches == nil {
		L.Push(lua.LNil)
		return 1
	}
	tbl := L.NewTable()
	for i, m := range matches {
		tbl.RawSetInt(i+1, lua.LString(m))
	}
	L.Push(tbl)
	return 1
}

// regexFindAll returns all non-overlapping matches.
// Lua: regex.find_all(pattern, str [, limit]) -> table
func regexFindAll(L *lua.LState) int {
	pattern := L.CheckString(1)
	str := L.CheckString(2)
	limit := L.OptInt(3, -1)
	re, err := regexp.Compile(pattern)
	if err != nil {
		L.ArgError(1, "invalid regex: "+err.Error())
		return 0
	}
	matches := re.FindAllString(str, limit)
	tbl := L.NewTable()
	for i, m := range matches {
		tbl.RawSetInt(i+1, lua.LString(m))
	}
	L.Push(tbl)
	return 1
}

// regexReplace replaces all matches of pattern in str with repl.
// Lua: regex.replace(pattern, str, repl) -> string
func regexReplace(L *lua.LState) int {
	pattern := L.CheckString(1)
	str := L.CheckString(2)
	repl := L.CheckString(3)
	re, err := regexp.Compile(pattern)
	if err != nil {
		L.ArgError(1, "invalid regex: "+err.Error())
		return 0
	}
	L.Push(lua.LString(re.ReplaceAllString(str, repl)))
	return 1
}

// regexSplit splits str by the regex pattern.
// Lua: regex.split(pattern, str [, limit]) -> table
func regexSplit(L *lua.LState) int {
	pattern := L.CheckString(1)
	str := L.CheckString(2)
	limit := L.OptInt(3, -1)
	re, err := regexp.Compile(pattern)
	if err != nil {
		L.ArgError(1, "invalid regex: "+err.Error())
		return 0
	}
	parts := re.Split(str, limit)
	tbl := L.NewTable()
	for i, p := range parts {
		tbl.RawSetInt(i+1, lua.LString(p))
	}
	L.Push(tbl)
	return 1
}

// regexIsValid checks whether a pattern compiles without error.
// Lua: regex.is_valid(pattern) -> bool
func regexIsValid(L *lua.LState) int {
	_, err := regexp.Compile(L.CheckString(1))
	L.Push(lua.LBool(err == nil))
	return 1
}
