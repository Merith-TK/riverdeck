// This file is part of the lualib package. See utils.go for the package doc.
package lualib

import (
	"time"

	lua "github.com/yuin/gopher-lua"
)

// RegisterTime preloads the "time" module into the given Lua state.
// Lua scripts access it via: local time = require("time")
func RegisterTime(L *lua.LState) {
	L.PreloadModule("time", timeLoader)
}

func timeLoader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"now":       timeNow,
		"timestamp": timeTimestamp,
		"format":    timeFormat,
		"parse":     timeParse,
		"date":      timeDate,
		"sleep":     timeSleep,
	})
	L.Push(mod)
	return 1
}

// timeNow returns current Unix timestamp (seconds).
// Lua: time.now() -> number
func timeNow(L *lua.LState) int {
	L.Push(lua.LNumber(time.Now().Unix()))
	return 1
}

// timeTimestamp is an alias for time.now().
// Lua: time.timestamp() -> number
func timeTimestamp(L *lua.LState) int {
	L.Push(lua.LNumber(time.Now().Unix()))
	return 1
}

// timeFormat formats a Unix timestamp using a Go layout string.
// Lua: time.format(timestamp, layout) -> string
func timeFormat(L *lua.LState) int {
	ts := L.CheckNumber(1)
	layout := L.CheckString(2)
	L.Push(lua.LString(time.Unix(int64(ts), 0).Format(layout)))
	return 1
}

// timeParse parses a formatted time string and returns a Unix timestamp.
// Lua: time.parse(layout, value) -> number, err
func timeParse(L *lua.LState) int {
	layout := L.CheckString(1)
	value := L.CheckString(2)
	t, err := time.Parse(layout, value)
	if err != nil {
		L.Push(lua.LNil)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LNumber(t.Unix()))
	L.Push(lua.LNil)
	return 2
}

// timeDate returns a table with individual date/time components.
// Lua: time.date([timestamp]) -> table{year,month,day,hour,minute,second,weekday,yearday}
func timeDate(L *lua.LState) int {
	ts := L.OptNumber(1, lua.LNumber(time.Now().Unix()))
	t := time.Unix(int64(ts), 0)

	tbl := L.NewTable()
	tbl.RawSetString("year", lua.LNumber(t.Year()))
	tbl.RawSetString("month", lua.LNumber(t.Month()))
	tbl.RawSetString("day", lua.LNumber(t.Day()))
	tbl.RawSetString("hour", lua.LNumber(t.Hour()))
	tbl.RawSetString("minute", lua.LNumber(t.Minute()))
	tbl.RawSetString("second", lua.LNumber(t.Second()))
	tbl.RawSetString("weekday", lua.LNumber(t.Weekday()))
	tbl.RawSetString("yearday", lua.LNumber(t.YearDay()))
	L.Push(tbl)
	return 1
}

// timeSleep sleeps for the given number of milliseconds.
// NOTE: This blocks the current goroutine. In background scripts, prefer
// system.sleep() which yields the coroutine to Go for cooperative scheduling.
// Lua: time.sleep(ms)
func timeSleep(L *lua.LState) int {
	time.Sleep(time.Duration(L.CheckNumber(1)) * time.Millisecond)
	return 0
}
