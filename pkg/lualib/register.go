// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import lua "github.com/yuin/gopher-lua"

// RegisterAll preloads every lualib module into L in a single call.
// This is the recommended way to wire the standard library from
// pkg/scripting/runner.go — adding a new lualib module only requires
// updating this list instead of editing both lualib and runner.
func RegisterAll(L *lua.LState) {
	// Core stdlib
	RegisterUtils(L)
	RegisterStrings(L)
	RegisterJSON(L)
	RegisterTime(L)
	RegisterLog(L)
	RegisterMath(L)

	// Encoding / hashing
	RegisterBase64(L)
	RegisterCrypto(L)

	// Pattern matching & paths
	RegisterRegex(L)
	RegisterPath(L)
	RegisterURL(L)
}

// RegisterAllWithLogPrefix is identical to RegisterAll but uses a custom
// prefix for the log module (e.g. "[script-name] ").
func RegisterAllWithLogPrefix(L *lua.LState, logPrefix string) {
	RegisterUtils(L)
	RegisterStrings(L)
	RegisterJSON(L)
	RegisterTime(L)
	RegisterLogWithPrefix(L, logPrefix)
	RegisterMath(L)

	RegisterBase64(L)
	RegisterCrypto(L)

	RegisterRegex(L)
	RegisterPath(L)
	RegisterURL(L)
}
