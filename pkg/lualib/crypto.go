// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"encoding/hex"

	lua "github.com/yuin/gopher-lua"
)

// RegisterCrypto preloads the "crypto" module into the given Lua state.
// Lua scripts access it via: local crypto = require("crypto")
//
// Provides basic hashing for cache keys, API signatures, and data integrity
// checks. Not intended for password hashing or key derivation.
func RegisterCrypto(L *lua.LState) {
	L.PreloadModule("crypto", cryptoLoader)
}

func cryptoLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"md5":    cryptoMD5,
		"sha1":   cryptoSHA1,
		"sha256": cryptoSHA256,
	})
	L.Push(mod)
	return 1
}

// cryptoMD5 returns the hex-encoded MD5 hash of a string.
// Lua: crypto.md5("hello") -> "5d41402abc4b2a76b9719d911017c592"
func cryptoMD5(L *lua.LState) int {
	h := md5.Sum([]byte(L.CheckString(1)))
	L.Push(lua.LString(hex.EncodeToString(h[:])))
	return 1
}

// cryptoSHA1 returns the hex-encoded SHA-1 hash of a string.
// Lua: crypto.sha1("hello") -> "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
func cryptoSHA1(L *lua.LState) int {
	h := sha1.Sum([]byte(L.CheckString(1)))
	L.Push(lua.LString(hex.EncodeToString(h[:])))
	return 1
}

// cryptoSHA256 returns the hex-encoded SHA-256 hash of a string.
// Lua: crypto.sha256("hello") -> "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
func cryptoSHA256(L *lua.LState) int {
	h := sha256.Sum256([]byte(L.CheckString(1)))
	L.Push(lua.LString(hex.EncodeToString(h[:])))
	return 1
}
