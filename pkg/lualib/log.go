// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"fmt"
	"log"
	"os"

	lua "github.com/yuin/gopher-lua"
)

// RegisterLog preloads the "log" module into the given Lua state.
// Lua scripts access it via: local log = require("log")
func RegisterLog(L *lua.LState) {
	L.PreloadModule("log", logLoader(log.New(os.Stdout, "[SCRIPT] ", log.LstdFlags)))
}

// RegisterLogWithPrefix preloads the "log" module using a custom logger prefix.
func RegisterLogWithPrefix(L *lua.LState, prefix string) {
	L.PreloadModule("log", logLoader(log.New(os.Stdout, prefix, log.LstdFlags)))
}

func logLoader(logger *log.Logger) lua.LGFunction {
	return func(L *lua.LState) int {
		mod := L.NewTable()
		L.SetFuncs(mod, map[string]lua.LGFunction{
			"info":   makeLogFunc(logger, "[INFO]"),
			"warn":   makeLogFunc(logger, "[WARN]"),
			"error":  makeLogFunc(logger, "[ERROR]"),
			"debug":  makeLogFunc(logger, "[DEBUG]"),
			"printf": logPrintf(logger),
			"print":  makeLogFunc(logger, ""),
		})
		L.Push(mod)
		return 1
	}
}

func makeLogFunc(logger *log.Logger, level string) lua.LGFunction {
	return func(L *lua.LState) int {
		msg := L.CheckString(1)
		if level == "" {
			logger.Println(msg)
		} else {
			logger.Println(level, msg)
		}
		return 0
	}
}

func logPrintf(logger *log.Logger) lua.LGFunction {
	return func(L *lua.LState) int {
		format := L.CheckString(1)
		args := make([]interface{}, L.GetTop()-1)
		for i := 2; i <= L.GetTop(); i++ {
			args[i-2] = luaArgToInterface(L.Get(i))
		}
		logger.Println(fmt.Sprintf(format, args...))
		return 0
	}
}

func luaArgToInterface(v lua.LValue) interface{} {
	switch val := v.(type) {
	case *lua.LNilType:
		return "nil"
	case lua.LBool:
		return bool(val)
	case lua.LNumber:
		return float64(val)
	case lua.LString:
		return string(val)
	case *lua.LTable:
		return "[table]"
	default:
		return val.String()
	}
}
