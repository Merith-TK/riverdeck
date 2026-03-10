// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"encoding/json"
	"fmt"

	lua "github.com/yuin/gopher-lua"
)

// RegisterJSON preloads the "json" module into the given Lua state.
// Lua scripts access it via: local json = require("json")
func RegisterJSON(L *lua.LState) {
	L.PreloadModule("json", jsonLoader)
}

func jsonLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"encode": jsonEncode,
		"decode": jsonDecode,
	})
	L.Push(mod)
	return 1
}

// jsonEncode encodes a Lua value to a JSON string.
// Lua: json.encode(value) -> string, err
func jsonEncode(L *lua.LState) int {
	goVal := luaToGo(L.Get(1))
	data, err := json.Marshal(goVal)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LString(string(data)))
	L.Push(lua.LNil)
	return 2
}

// jsonDecode decodes a JSON string into a Lua value.
// Lua: json.decode(str) -> value, err
func jsonDecode(L *lua.LState) int {
	var result interface{}
	if err := json.Unmarshal([]byte(L.CheckString(1)), &result); err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(goToLua(L, result))
	L.Push(lua.LNil)
	return 2
}

// luaToGo converts a Lua value to a Go value suitable for json.Marshal.
func luaToGo(v lua.LValue) interface{} {
	switch val := v.(type) {
	case *lua.LNilType:
		return nil
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		// Detect array vs object: if all keys are consecutive integers starting at 1, treat as array.
		maxIdx := 0
		isArr := true
		val.ForEach(func(k, _ lua.LValue) {
			n, ok := k.(lua.LNumber)
			if !ok || float64(n) != float64(int(n)) || int(n) <= 0 {
				isArr = false
				return
			}
			if int(n) > maxIdx {
				maxIdx = int(n)
			}
		})
		if isArr && maxIdx > 0 {
			arr := make([]interface{}, maxIdx)
			for i := 1; i <= maxIdx; i++ {
				arr[i-1] = luaToGo(val.RawGetInt(i))
			}
			return arr
		}
		obj := make(map[string]interface{})
		val.ForEach(func(k, v lua.LValue) {
			if k.Type() == lua.LTString {
				obj[k.String()] = luaToGo(v)
			}
		})
		return obj
	default:
		return fmt.Sprintf("%v", v)
	}
}

// goToLua converts a Go value (from json.Unmarshal) back to a Lua value.
func goToLua(L *lua.LState, v interface{}) lua.LValue {
	switch val := v.(type) {
	case nil:
		return lua.LNil
	case bool:
		return lua.LBool(val)
	case float64:
		return lua.LNumber(val)
	case string:
		return lua.LString(val)
	case []interface{}:
		tbl := L.NewTable()
		for i, item := range val {
			tbl.RawSetInt(i+1, goToLua(L, item))
		}
		return tbl
	case map[string]interface{}:
		tbl := L.NewTable()
		for k, item := range val {
			tbl.RawSetString(k, goToLua(L, item))
		}
		return tbl
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}
