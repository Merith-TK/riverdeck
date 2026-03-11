// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"math"
	"math/rand"

	lua "github.com/yuin/gopher-lua"
)

// RegisterMath preloads the "math" module into the given Lua state.
// Lua scripts access it via: local math = require("math")
//
// Supplements gopher-lua's built-in math table with clamp, round, lerp, and
// a seeded random generator — functions that are especially useful for
// Stream Deck colour/brightness calculations and animation easing.
func RegisterMath(L *lua.LState) {
	L.PreloadModule("math", mathLoader)
}

func mathLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"floor":       mathFloor,
		"ceil":        mathCeil,
		"abs":         mathAbs,
		"round":       mathRound,
		"min":         mathMin,
		"max":         mathMax,
		"clamp":       mathClamp,
		"random":      mathRandom,
		"random_seed": mathRandomSeed,
		"sqrt":        mathSqrt,
		"pow":         mathPow,
		"log":         mathLog,
		"sin":         mathSin,
		"cos":         mathCos,
		"pi":          mathPi,
		"huge":        mathHuge,
		"lerp":        mathLerp,
	})
	L.Push(mod)
	return 1
}

// mathFloor returns the largest integer ≤ x.
// Lua: math.floor(x) -> number
func mathFloor(L *lua.LState) int {
	L.Push(lua.LNumber(math.Floor(float64(L.CheckNumber(1)))))
	return 1
}

// mathCeil returns the smallest integer ≥ x.
// Lua: math.ceil(x) -> number
func mathCeil(L *lua.LState) int {
	L.Push(lua.LNumber(math.Ceil(float64(L.CheckNumber(1)))))
	return 1
}

// mathAbs returns the absolute value of x.
// Lua: math.abs(x) -> number
func mathAbs(L *lua.LState) int {
	L.Push(lua.LNumber(math.Abs(float64(L.CheckNumber(1)))))
	return 1
}

// mathRound rounds x to the nearest integer (half away from zero).
// Lua: math.round(x) -> number
func mathRound(L *lua.LState) int {
	L.Push(lua.LNumber(math.Round(float64(L.CheckNumber(1)))))
	return 1
}

// mathMin returns the smallest value among the arguments.
// Lua: math.min(a, b, ...) -> number
func mathMin(L *lua.LState) int {
	n := L.GetTop()
	if n == 0 {
		L.ArgError(1, "expected at least one argument")
		return 0
	}
	result := float64(L.CheckNumber(1))
	for i := 2; i <= n; i++ {
		v := float64(L.CheckNumber(i))
		if v < result {
			result = v
		}
	}
	L.Push(lua.LNumber(result))
	return 1
}

// mathMax returns the largest value among the arguments.
// Lua: math.max(a, b, ...) -> number
func mathMax(L *lua.LState) int {
	n := L.GetTop()
	if n == 0 {
		L.ArgError(1, "expected at least one argument")
		return 0
	}
	result := float64(L.CheckNumber(1))
	for i := 2; i <= n; i++ {
		v := float64(L.CheckNumber(i))
		if v > result {
			result = v
		}
	}
	L.Push(lua.LNumber(result))
	return 1
}

// mathClamp constrains x to the range [lo, hi].
// Lua: math.clamp(x, lo, hi) -> number
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

// mathRandom returns a pseudo-random number.
// No args: float in [0,1). One arg (n): int in [1,n]. Two args (m,n): int in [m,n].
// Lua: math.random([m [, n]]) -> number
func mathRandom(L *lua.LState) int {
	switch L.GetTop() {
	case 0:
		L.Push(lua.LNumber(rand.Float64()))
	case 1:
		n := L.CheckInt(1)
		L.Push(lua.LNumber(rand.Intn(n) + 1))
	default:
		m := L.CheckInt(1)
		n := L.CheckInt(2)
		L.Push(lua.LNumber(rand.Intn(n-m+1) + m))
	}
	return 1
}

// mathRandomSeed seeds the random number generator.
// Lua: math.random_seed(seed)
func mathRandomSeed(L *lua.LState) int {
	rand.Seed(int64(L.CheckNumber(1)))
	return 0
}

// mathSqrt returns the square root of x.
// Lua: math.sqrt(x) -> number
func mathSqrt(L *lua.LState) int {
	L.Push(lua.LNumber(math.Sqrt(float64(L.CheckNumber(1)))))
	return 1
}

// mathPow returns x raised to the power y.
// Lua: math.pow(x, y) -> number
func mathPow(L *lua.LState) int {
	L.Push(lua.LNumber(math.Pow(float64(L.CheckNumber(1)), float64(L.CheckNumber(2)))))
	return 1
}

// mathLog returns the natural logarithm of x (or log base b if given).
// Lua: math.log(x [, base]) -> number
func mathLog(L *lua.LState) int {
	x := float64(L.CheckNumber(1))
	if L.GetTop() >= 2 {
		base := float64(L.CheckNumber(2))
		L.Push(lua.LNumber(math.Log(x) / math.Log(base)))
	} else {
		L.Push(lua.LNumber(math.Log(x)))
	}
	return 1
}

// mathSin returns the sine of x (in radians).
// Lua: math.sin(x) -> number
func mathSin(L *lua.LState) int {
	L.Push(lua.LNumber(math.Sin(float64(L.CheckNumber(1)))))
	return 1
}

// mathCos returns the cosine of x (in radians).
// Lua: math.cos(x) -> number
func mathCos(L *lua.LState) int {
	L.Push(lua.LNumber(math.Cos(float64(L.CheckNumber(1)))))
	return 1
}

// mathPi returns π.
// Lua: math.pi() -> number
func mathPi(L *lua.LState) int {
	L.Push(lua.LNumber(math.Pi))
	return 1
}

// mathHuge returns +Inf.
// Lua: math.huge() -> number
func mathHuge(L *lua.LState) int {
	L.Push(lua.LNumber(math.Inf(1)))
	return 1
}

// mathLerp linearly interpolates between a and b by t ∈ [0,1].
// Lua: math.lerp(a, b, t) -> number
func mathLerp(L *lua.LState) int {
	a := float64(L.CheckNumber(1))
	b := float64(L.CheckNumber(2))
	t := float64(L.CheckNumber(3))
	L.Push(lua.LNumber(a + (b-a)*t))
	return 1
}
