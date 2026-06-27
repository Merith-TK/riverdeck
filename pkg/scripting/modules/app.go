// This file is part of the modules package. See system.go for the package doc.
package modules

import lua "github.com/yuin/gopher-lua"

// AppModule exposes app-mode navigation and layout control to .app.lua scripts.
// It is registered only inside AppRunner Lua VMs, never in regular ScriptRunner VMs.
type AppModule struct {
	onNavigate    func(page string)
	onBack        func()
	onExit        func()
	onCurrentPage func() string
	onUpdateKey   func(page string, slot int, tbl *lua.LTable)
	onDefaultKey  func(key int)
	onRefresh     func()
}

// NewAppModule creates an AppModule with the given callbacks.
// All callbacks are optional; nil callbacks are silently ignored.
func NewAppModule(
	onNavigate func(page string),
	onBack func(),
	onExit func(),
	onCurrentPage func() string,
	onUpdateKey func(page string, slot int, tbl *lua.LTable),
	onDefaultKey func(key int),
	onRefresh func(),
) *AppModule {
	return &AppModule{
		onNavigate:    onNavigate,
		onBack:        onBack,
		onExit:        onExit,
		onCurrentPage: onCurrentPage,
		onUpdateKey:   onUpdateKey,
		onDefaultKey:  onDefaultKey,
		onRefresh:     onRefresh,
	}
}

// Loader returns the Lua module loader for the "app" module.
func (m *AppModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"navigate":     m.appNavigate,
		"back":         m.appBack,
		"exit":         m.appExit,
		"current_page": m.appCurrentPage,
		"update_key":   m.appUpdateKey,
		"default_key":  m.appDefaultKey,
		"refresh":      m.appRefresh,
	})
	L.Push(mod)
	return 1
}

// appNavigate pushes a new page onto the nav stack.
// Lua: app.navigate("page_name")
func (m *AppModule) appNavigate(L *lua.LState) int {
	page := L.CheckString(1)
	if m.onNavigate != nil {
		m.onNavigate(page)
	}
	return 0
}

// appBack pops the nav stack; exits app if at the root page.
// Lua: app.back()
func (m *AppModule) appBack(L *lua.LState) int {
	if m.onBack != nil {
		m.onBack()
	}
	return 0
}

// appExit exits app mode and returns to normal riverdeck navigation.
// Lua: app.exit()
func (m *AppModule) appExit(L *lua.LState) int {
	if m.onExit != nil {
		m.onExit()
	}
	return 0
}

// appCurrentPage returns the current page name.
// Lua: app.current_page() -> string
func (m *AppModule) appCurrentPage(L *lua.LState) int {
	if m.onCurrentPage != nil {
		L.Push(lua.LString(m.onCurrentPage()))
	} else {
		L.Push(lua.LString(""))
	}
	return 1
}

// appUpdateKey mutates an entry in the page table at runtime.
// Lua: app.update_key("page", slot, {text="...", color={r,g,b}, ...})
func (m *AppModule) appUpdateKey(L *lua.LState) int {
	page := L.CheckString(1)
	slot := L.CheckInt(2)
	tbl, ok := L.Get(3).(*lua.LTable)
	if !ok {
		return 0
	}
	if m.onUpdateKey != nil {
		m.onUpdateKey(page, slot, tbl)
	}
	return 0
}

// appDefaultKey runs the page-table action/folder dispatch for a key.
// Use inside app_key to fall back to declarative behaviour for unhandled keys.
// Lua: app.default_key(key, state)  -- state arg is accepted but ignored
func (m *AppModule) appDefaultKey(L *lua.LState) int {
	key := L.CheckInt(1)
	if m.onDefaultKey != nil {
		m.onDefaultKey(key)
	}
	return 0
}

// appRefresh forces an immediate passive re-render (like system.refresh).
// Lua: app.refresh()
func (m *AppModule) appRefresh(L *lua.LState) int {
	if m.onRefresh != nil {
		m.onRefresh()
	}
	return 0
}
