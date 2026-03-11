package modules

// store.go - provides a thread-safe global key-value store to Lua scripts.
//
// A single StoreModule instance is created by ScriptManager and its Loader
// method is preloaded into every ScriptRunner.  Because the backing sync.Map
// lives on the shared Go struct (not inside any lua.LState), values written by
// one script are immediately visible to all other scripts, even across
// independent Lua states.
//
// Lua usage:
//
//	local store = require('store')
//
//	-- write
//	store.set("obs.streaming", true)
//	store.set("volume.level",  75)
//	store.set("mode",          "gaming")
//
//	-- read
//	local level = store.get("volume.level")   -- returns number|string|bool|nil
//	if store.has("mode") then ... end
//
//	-- remove
//	store.delete("mode")
//
//	-- iterate
//	for _, k in ipairs(store.keys()) do
//	    print(k, store.get(k))
//	end
//
// Supported value types: string, number (stored as float64), bool.
// Tables and functions cannot be stored across Lua states.

import (
	"fmt"
	"sort"
	"sync"

	lua "github.com/yuin/gopher-lua"
)

// StoreModule is the shared cross-script key-value store.
// Create exactly one instance per ScriptManager via NewStoreModule, then
// pass the same instance to every ScriptRunner so they all share one map.
type StoreModule struct {
	data sync.Map // map[string]interface{}  -  string | float64 | bool
}

// NewStoreModule creates a new, empty shared store.
func NewStoreModule() *StoreModule {
	return &StoreModule{}
}

// Loader is the gopher-lua module loader. Pass it to L.PreloadModule("store", ...).
func (m *StoreModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"get":    m.storeGet,
		"set":    m.storeSet,
		"delete": m.storeDelete,
		"has":    m.storeHas,
		"keys":   m.storeKeys,
	})
	L.Push(mod)
	return 1
}

// storeGet retrieves a value by key.
// Lua: local v = store.get("key")  ->  string|number|boolean|nil
func (m *StoreModule) storeGet(L *lua.LState) int {
	key := L.CheckString(1)
	val, ok := m.data.Load(key)
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(goToLua(val))
	return 1
}

// storeSet stores a value under a key, replacing any previous value.
// Lua: store.set("key", value)
func (m *StoreModule) storeSet(L *lua.LState) int {
	key := L.CheckString(1)
	val := L.CheckAny(2)
	m.data.Store(key, luaToGo(val))
	return 0
}

// storeDelete removes a key. No-op when the key does not exist.
// Lua: store.delete("key")
func (m *StoreModule) storeDelete(L *lua.LState) int {
	key := L.CheckString(1)
	m.data.Delete(key)
	return 0
}

// storeHas returns true when the key exists and is not nil.
// Lua: if store.has("key") then ... end
func (m *StoreModule) storeHas(L *lua.LState) int {
	key := L.CheckString(1)
	_, ok := m.data.Load(key)
	L.Push(lua.LBool(ok))
	return 1
}

// storeKeys returns a sorted table containing all keys currently in the store.
// Lua: for _, k in ipairs(store.keys()) do print(k) end
func (m *StoreModule) storeKeys(L *lua.LState) int {
	var keys []string
	m.data.Range(func(k, _ interface{}) bool {
		if s, ok := k.(string); ok {
			keys = append(keys, s)
		}
		return true
	})
	sort.Strings(keys)

	tbl := L.NewTable()
	for i, k := range keys {
		tbl.RawSetInt(i+1, lua.LString(k))
	}
	L.Push(tbl)
	return 1
}

// goToLua converts a stored Go value back to a lua.LValue.
func goToLua(val interface{}) lua.LValue {
	switch v := val.(type) {
	case string:
		return lua.LString(v)
	case bool:
		return lua.LBool(v)
	case float64:
		return lua.LNumber(v)
	case int:
		return lua.LNumber(float64(v))
	case int64:
		return lua.LNumber(float64(v))
	default:
		return lua.LString(fmt.Sprintf("%v", v))
	}
}

// luaToGo converts a Lua value to a Go primitive suitable for sync.Map storage.
func luaToGo(val lua.LValue) interface{} {
	switch v := val.(type) {
	case lua.LString:
		return string(v)
	case lua.LBool:
		return bool(v)
	case lua.LNumber:
		return float64(v)
	default:
		// Tables and functions cannot be meaningfully shared across Lua states;
		// fall back to their string representation.
		return v.String()
	}
}
