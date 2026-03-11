package modules

// config.go provides a read-only configuration module for Lua scripts.
//
// Every script can require('config') to access per-button configuration
// values.  The module exposes three functions:
//
//   local config = require('config')
//   config.get("key")      -- merged value (override wins over default)
//   config.all()           -- table of all merged key/value pairs
//   config.schema()        -- array of {key, label, type, default, description}
//
// The backing data comes from two sources depending on navigation mode:
//
// Folder mode:
//   A sibling .config.json file next to the Lua script.
//   Format: {"schema": [...], "overrides": {...}}
//
// Layout mode:
//   The package template's MetadataSchema provides defaults,
//   and the button's Metadata map provides overrides.
//
// Both are resolved by ScriptManager and injected into the ConfigModule
// before the script runs.

import (
	lua "github.com/yuin/gopher-lua"
)

// ConfigField mirrors scripting.MetadataField for use within the module
// without importing the parent package.
type ConfigField struct {
	Key         string
	Label       string
	Type        string
	Default     string
	Description string
}

// ConfigModule is a read-only per-script configuration store.
type ConfigModule struct {
	schema   []ConfigField
	defaults map[string]string
	merged   map[string]string // defaults + overrides
}

// NewConfigModule creates a ConfigModule from schema fields and user overrides.
// The merged map is pre-computed: for every schema key, the override value
// wins if present; otherwise the schema default is used.
func NewConfigModule(schema []ConfigField, overrides map[string]string) *ConfigModule {
	defaults := make(map[string]string, len(schema))
	merged := make(map[string]string, len(schema))

	for _, f := range schema {
		defaults[f.Key] = f.Default
		merged[f.Key] = f.Default
	}

	for k, v := range overrides {
		merged[k] = v
	}

	return &ConfigModule{
		schema:   schema,
		defaults: defaults,
		merged:   merged,
	}
}

// Loader is the gopher-lua module loader.
// Preload as: L.PreloadModule("config", cfgMod.Loader)
func (m *ConfigModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"get":    m.cfgGet,
		"all":    m.cfgAll,
		"schema": m.cfgSchema,
	})
	L.Push(mod)
	return 1
}

// cfgGet returns a single merged config value by key.
// Lua: config.get("key") -> string|nil
func (m *ConfigModule) cfgGet(L *lua.LState) int {
	key := L.CheckString(1)
	val, ok := m.merged[key]
	if !ok {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(val))
	return 1
}

// cfgAll returns a table of all merged config values.
// Lua: config.all() -> {key1 = "value1", key2 = "value2", ...}
func (m *ConfigModule) cfgAll(L *lua.LState) int {
	tbl := L.NewTable()
	for k, v := range m.merged {
		tbl.RawSetString(k, lua.LString(v))
	}
	L.Push(tbl)
	return 1
}

// cfgSchema returns the full schema as an array of tables.
// Lua: config.schema() -> {{key="k", label="L", type="text", default="", description=""}, ...}
func (m *ConfigModule) cfgSchema(L *lua.LState) int {
	arr := L.NewTable()
	for _, f := range m.schema {
		entry := L.NewTable()
		entry.RawSetString("key", lua.LString(f.Key))
		entry.RawSetString("label", lua.LString(f.Label))
		entry.RawSetString("type", lua.LString(f.Type))
		entry.RawSetString("default", lua.LString(f.Default))
		entry.RawSetString("description", lua.LString(f.Description))
		arr.Append(entry)
	}
	L.Push(arr)
	return 1
}
