// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"net/url"

	lua "github.com/yuin/gopher-lua"
)

// RegisterURL preloads the "url" module into the given Lua state.
// Lua scripts access it via: local url = require("url")
//
// Provides URL parsing, encoding, and query-string building -- handy for
// constructing API endpoints from Lua scripts.
func RegisterURL(L *lua.LState) {
	L.PreloadModule("url", urlLoader)
}

func urlLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"parse":        urlParse,
		"encode":       urlEncode,
		"decode":       urlDecode,
		"query_encode": urlQueryEncode,
		"query_decode": urlQueryDecode,
	})
	L.Push(mod)
	return 1
}

// urlParse parses a raw URL string into a table.
// Lua: url.parse("https://example.com:8080/path?q=1#frag")
//
//	-> { scheme="https", host="example.com:8080", hostname="example.com",
//	     port="8080", path="/path", query="q=1", fragment="frag" }
func urlParse(L *lua.LState) int {
	raw := L.CheckString(1)
	u, err := url.Parse(raw)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	tbl.RawSetString("scheme", lua.LString(u.Scheme))
	tbl.RawSetString("host", lua.LString(u.Host))
	tbl.RawSetString("hostname", lua.LString(u.Hostname()))
	tbl.RawSetString("port", lua.LString(u.Port()))
	tbl.RawSetString("path", lua.LString(u.Path))
	tbl.RawSetString("query", lua.LString(u.RawQuery))
	tbl.RawSetString("fragment", lua.LString(u.Fragment))
	if u.User != nil {
		tbl.RawSetString("user", lua.LString(u.User.Username()))
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}

// urlEncode percent-encodes a string for safe inclusion in a URL path segment.
// Lua: url.encode("hello world") -> "hello%20world"
func urlEncode(L *lua.LState) int {
	L.Push(lua.LString(url.PathEscape(L.CheckString(1))))
	return 1
}

// urlDecode percent-decodes a URL-encoded string.
// Lua: url.decode("hello%20world") -> "hello world", nil
func urlDecode(L *lua.LState) int {
	s, err := url.PathUnescape(L.CheckString(1))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(s))
	L.Push(lua.LNil)
	return 2
}

// urlQueryEncode encodes a table of key-value pairs as a URL query string.
// Lua: url.query_encode({q="hello", page="1"}) -> "page=1&q=hello"
func urlQueryEncode(L *lua.LState) int {
	tbl := L.CheckTable(1)
	params := url.Values{}
	tbl.ForEach(func(k, v lua.LValue) {
		params.Set(k.String(), v.String())
	})
	L.Push(lua.LString(params.Encode()))
	return 1
}

// urlQueryDecode parses a query string into a table.
// Lua: url.query_decode("q=hello&page=1") -> {q="hello", page="1"}, nil
func urlQueryDecode(L *lua.LState) int {
	qs := L.CheckString(1)
	vals, err := url.ParseQuery(qs)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	tbl := L.NewTable()
	for k, v := range vals {
		if len(v) == 1 {
			tbl.RawSetString(k, lua.LString(v[0]))
		} else {
			arr := L.NewTable()
			for i, item := range v {
				arr.RawSetInt(i+1, lua.LString(item))
			}
			tbl.RawSetString(k, arr)
		}
	}
	L.Push(tbl)
	L.Push(lua.LNil)
	return 2
}
