// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"math"

	lua "github.com/yuin/gopher-lua"
)

// RegisterMath preloads the "mathx" module into the given Lua state.
// Lua scripts access it via: local mathx = require("mathx")
//
// Provides clamp, round, and lerp — the three useful math functions that
// gopher-lua's built-in math library does not include. For everything else
// (floor, ceil, abs, min, max, sin, cos, sqrt, random, pi, huge, …) use
// the built-in math global directly.
func RegisterMath(L *lua.LState) {
	L.PreloadModule("mathx", mathLoader)
}

func mathLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"clamp": mathClamp,
		"round": mathRound,
		"lerp":  mathLerp,
	})
	L.Push(mod)
	return 1
}

// mathClamp constrains x to the range [lo, hi].
// Lua: mathx.clamp(x, lo, hi) -> number
func mathClamp(L *lua.LState) int {
	x := float64(L.CheckNumber(1))
	lo := float64(L.CheckNumber(2))
	hi := float64(L.CheckNumber(3))
	if x < lo {
		x = lo
	} else if x > hi {
		x = hi
	}
	L.Push(lua.LNumber(x))
	return 1
}

// mathRound rounds x to the nearest integer (half away from zero).
// Lua: mathx.round(x) -> number
func mathRound(L *lua.LState) int {
	L.Push(lua.LNumber(math.Round(float64(L.CheckNumber(1)))))
	return 1
}

// mathLerp linearly interpolates between a and b by t ∈ [0,1].
// Lua: mathx.lerp(a, b, t) -> number
func mathLerp(L *lua.LState) int {
	a := float64(L.CheckNumber(1))
	b := float64(L.CheckNumber(2))
	t := float64(L.CheckNumber(3))
	L.Push(lua.LNumber(a + (b-a)*t))
	return 1
}
