// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"encoding/base64"

	lua "github.com/yuin/gopher-lua"
)

// RegisterBase64 preloads the "base64" module into the given Lua state.
// Lua scripts access it via: local b64 = require("base64")
//
// Useful for encoding API authentication headers, embedding binary data
// in JSON payloads, and similar integration tasks.
func RegisterBase64(L *lua.LState) {
	L.PreloadModule("base64", base64Loader)
}

func base64Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"encode":     base64Encode,
		"decode":     base64Decode,
		"url_encode": base64URLEncode,
		"url_decode": base64URLDecode,
	})
	L.Push(mod)
	return 1
}

// base64Encode encodes a string to standard base64.
// Lua: base64.encode("hello") -> "aGVsbG8="
func base64Encode(L *lua.LState) int {
	L.Push(lua.LString(base64.StdEncoding.EncodeToString([]byte(L.CheckString(1)))))
	return 1
}

// base64Decode decodes a standard base64 string.
// Lua: base64.decode("aGVsbG8=") -> "hello", nil
func base64Decode(L *lua.LState) int {
	data, err := base64.StdEncoding.DecodeString(L.CheckString(1))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}

// base64URLEncode encodes a string to URL-safe base64 (no padding).
// Lua: base64.url_encode("hello") -> "aGVsbG8"
func base64URLEncode(L *lua.LState) int {
	L.Push(lua.LString(base64.RawURLEncoding.EncodeToString([]byte(L.CheckString(1)))))
	return 1
}

// base64URLDecode decodes a URL-safe base64 string (no padding).
// Lua: base64.url_decode("aGVsbG8") -> "hello", nil
func base64URLDecode(L *lua.LState) int {
	data, err := base64.RawURLEncoding.DecodeString(L.CheckString(1))
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}
