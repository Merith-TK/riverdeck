// This file is part of the modules package. See system.go for the package doc.
package modules

// config.go provides the per-script configuration module for Lua scripts.
//
// Scripts can access configuration via:
//
//	-- Legacy API (still supported, maps to riverdeck.config)
//	local config = require('config')
//	config.get("key")      -- merged value (override wins over default)
//	config.all()           -- table of all merged key/value pairs
//	config.schema()        -- array of {key, label, type, default, description}
//
//	-- New riverdeck.config API
//	local cfg = require("riverdeck.config")
//	cfg.script.defaultdata = { volume = 5, foo = "bar" }
//	cfg.script.sync()           -- reads .config.json, fills from defaultdata
//	local vol = cfg.script.data.volume
//	cfg.script.data.volume = 10
//	cfg.script.save()           -- writes overrides back to disk
//
// Both APIs are backed by the same ConfigModule. The `script` sub-table
// exposes the new API; the module itself exports the legacy `get`/`all`/`schema`
// functions for backward compatibility.
//
// The backing data comes from two sources depending on navigation mode:
//
// Folder mode:
//
//	A sibling .config.json file next to the Lua script.
//	Format: {"schema": [...], "overrides": {...}}
//
// Layout mode:
//
//	The package template's MetadataSchema provides defaults,
//	and the button's Metadata map provides overrides.
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

// ConfigModule is a per-script configuration store supporting both the legacy
// and the new riverdeck.config API.
type ConfigModule struct {
	schema   []ConfigField
	defaults map[string]string
	merged   map[string]string // defaults + overrides

	// scriptPath is the absolute path of the owning script. Used by sync()
	// and save() to locate the sibling .config.json.
	scriptPath string
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

// SetScriptPath stores the script path for use by sync()/save().
func (m *ConfigModule) SetScriptPath(path string) {
	m.scriptPath = path
}

// Loader is the gopher-lua module loader for both "config" (legacy) and
// "riverdeck.config" (new) module names.
// Preload as: L.PreloadModule("config", cfgMod.Loader)
//             L.PreloadModule("riverdeck.config", cfgMod.Loader)
func (m *ConfigModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		// Legacy API
		"get":    m.cfgGet,
		"all":    m.cfgAll,
		"schema": m.cfgSchema,
	})

	// New API: cfg.script sub-table
	scriptTbl := L.NewTable()

	// cfg.script.defaultdata — writable table set by the script
	L.SetField(scriptTbl, "defaultdata", L.NewTable())

	// cfg.script.data — read/write data table (populated after sync())
	dataTbl := L.NewTable()
	for k, v := range m.merged {
		dataTbl.RawSetString(k, lua.LString(v))
	}
	L.SetField(scriptTbl, "data", dataTbl)

	// cfg.script.sync() — merge defaultdata + disk overrides into data
	L.SetField(scriptTbl, "sync", L.NewFunction(m.makeSync(L, scriptTbl)))

	// cfg.script.save() — write data back as overrides
	L.SetField(scriptTbl, "save", L.NewFunction(m.makeSave(L, scriptTbl)))

	L.SetField(mod, "script", scriptTbl)

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

// makeSync returns a Lua function that:
//  1. Reads script's defaultdata table (declared by the script).
//  2. Merges with any disk overrides (from .config.json in folder mode,
//     or from the existing merged values already set in layout mode).
//  3. Writes the result into cfg.script.data.
func (m *ConfigModule) makeSync(L *lua.LState, scriptTbl *lua.LTable) lua.LGFunction {
	return func(L *lua.LState) int {
		// Read defaultdata declared by the script.
		defaults := L.GetField(scriptTbl, "defaultdata")
		data := L.NewTable()

		// First, apply defaults from defaultdata.
		if dt, ok := defaults.(*lua.LTable); ok {
			dt.ForEach(func(k, v lua.LValue) {
				data.RawSet(k, v)
			})
		}

		// Then, overlay with the pre-merged values from the Go side.
		// These already incorporate schema defaults + overrides.
		for k, v := range m.merged {
			data.RawSetString(k, lua.LString(v))
		}

		L.SetField(scriptTbl, "data", data)
		return 0
	}
}

// makeSave returns a Lua function that writes cfg.script.data back to
// the module's merged map (in-memory only for layout mode; folder mode
// would need disk write, which is left for future implementation).
func (m *ConfigModule) makeSave(L *lua.LState, scriptTbl *lua.LTable) lua.LGFunction {
	return func(L *lua.LState) int {
		data := L.GetField(scriptTbl, "data")
		if dt, ok := data.(*lua.LTable); ok {
			dt.ForEach(func(k, v lua.LValue) {
				if ks, ok := k.(lua.LString); ok {
					m.merged[string(ks)] = v.String()
				}
			})
		}
		return 0
	}
}
