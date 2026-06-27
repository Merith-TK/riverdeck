package scripting

import (
	"fmt"
	"path/filepath"
	"sync"

	"github.com/merith-tk/riverdeck/pkg/lualib"
	"github.com/merith-tk/riverdeck/pkg/resolver"
	"github.com/merith-tk/riverdeck/pkg/scripting/modules"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	lua "github.com/yuin/gopher-lua"
)

// AppPage maps key slot indices to their AppEntry definitions.
type AppPage = map[int]*AppEntry

// AppEntry defines a single key's static appearance and press behaviour
// within an app page. Dynamic keys can additionally supply a Passive function.
type AppEntry struct {
	Text       string
	Color      [3]int
	TextColor  [3]int
	Icon       string       // resolved absolute path
	FolderPage string       // if non-empty, navigate here on press
	Action     *lua.LFunction // called on press (owned by AppRunner's Lua VM)
	Passive    *lua.LFunction // called per passive frame for dynamic display
}

// AppRunner loads and executes a .app.lua file, managing its own Lua VM,
// page table, and navigation stack. It is independent of ScriptRunner.
//
// Lock ordering: luaMu → mu (never reverse).
type AppRunner struct {
	luaMu sync.Mutex  // serialises all Lua VM operations
	mu    sync.RWMutex // protects pages, navStack, startPage

	scriptPath string
	configDir  string
	packages   []resolver.PackageInfo

	L      *lua.LState
	state  *lua.LTable
	module *lua.LTable

	pages     map[string]AppPage
	navStack  []string
	startPage string

	hasAppPassive bool
	hasAppKey     bool

	device     streamdeck.DeviceIface
	onExit     func() // set by ScriptManager.EnterAppMode
	onRerender func() // set by ScriptManager.EnterAppMode
}

// NewAppRunner creates an AppRunner for the given .app.lua file path.
// Callbacks (onExit, onRerender) are wired later by ScriptManager.EnterAppMode.
func NewAppRunner(dev streamdeck.DeviceIface, scriptPath, configDir string, packages []resolver.PackageInfo) *AppRunner {
	return &AppRunner{
		scriptPath: scriptPath,
		configDir:  configDir,
		packages:   packages,
		device:     dev,
		pages:      make(map[string]AppPage),
	}
}

// Load executes the .app.lua file, parses M.pages, detects function overrides,
// and initialises the navigation stack. Must be called before any Render/Handle.
func (r *AppRunner) Load() error {
	r.L = lua.NewState()
	r.state = r.L.NewTable()
	r.L.SetGlobal("state", r.state)

	// Register standard modules. systemMod uses a closure so that onRerender
	// wired after Load() is still picked up at call time.
	shellMod := modules.NewShellModule()
	httpMod := modules.NewHTTPModule()
	systemMod := modules.NewSystemModule(func() {
		if r.onRerender != nil {
			r.onRerender()
		}
	})
	sdMod := modules.NewStreamDeckModule(r.device)
	fileMod := modules.NewFileModule()
	storeMod := modules.NewStoreModule()

	r.L.PreloadModule("shell", shellMod.Loader)
	r.L.PreloadModule("http", httpMod.Loader)
	r.L.PreloadModule("system", systemMod.Loader)
	r.L.PreloadModule("streamdeck", sdMod.Loader)
	r.L.PreloadModule("file", fileMod.Loader)
	r.L.PreloadModule("store", storeMod.Loader)
	lualib.RegisterAll(r.L)

	// Register the app module — callbacks use r's methods so that onExit/onRerender
	// set after Load() are still honoured at call time.
	appMod := modules.NewAppModule(
		r.luaNavigate,
		r.luaBack,
		r.luaExit,
		r.currentPageNoLock,
		r.luaUpdateKey,
		r.defaultKey,
		func() {
			if r.onRerender != nil {
				r.onRerender()
			}
		},
	)
	r.L.PreloadModule("app", appMod.Loader)

	r.L.SetGlobal("SCRIPT_PATH", lua.LString(r.scriptPath))
	r.L.SetGlobal("CONFIG_DIR", lua.LString(r.configDir))

	if err := r.L.DoFile(r.scriptPath); err != nil {
		r.L.Close()
		r.L = nil
		return fmt.Errorf("load %s: %w", filepath.Base(r.scriptPath), err)
	}

	result := r.L.Get(-1)
	if result.Type() != lua.LTTable {
		r.L.Close()
		r.L = nil
		return fmt.Errorf("%s must return a table (got %s)", filepath.Base(r.scriptPath), result.Type())
	}
	r.L.Pop(1)
	r.module = result.(*lua.LTable)

	r.hasAppPassive = r.module.RawGetString("app_passive").Type() == lua.LTFunction
	r.hasAppKey = r.module.RawGetString("app_key").Type() == lua.LTFunction

	if sp := r.module.RawGetString("start_page"); sp.Type() == lua.LTString {
		r.startPage = sp.String()
	}
	if r.startPage == "" {
		r.startPage = "root"
	}

	if pagesVal := r.module.RawGetString("pages"); pagesVal.Type() == lua.LTTable {
		r.parsePages(pagesVal.(*lua.LTable))
	}

	r.navStack = []string{r.startPage}
	r.L.SetField(r.state, "_page", lua.LString(r.startPage))

	fmt.Printf("[*] App loaded: %s (app_passive=%v app_key=%v pages=%d start=%q)\n",
		filepath.Base(r.scriptPath), r.hasAppPassive, r.hasAppKey, len(r.pages), r.startPage)

	return nil
}

// Close releases the Lua VM.
func (r *AppRunner) Close() {
	r.luaMu.Lock()
	defer r.luaMu.Unlock()
	if r.L != nil {
		r.L.Close()
		r.L = nil
	}
}

// ── Page table parsing ────────────────────────────────────────────────────────

func (r *AppRunner) parsePages(pagesTbl *lua.LTable) {
	pagesTbl.ForEach(func(key, val lua.LValue) {
		pageName, ok := key.(lua.LString)
		if !ok || val.Type() != lua.LTTable {
			return
		}
		page := make(AppPage)
		val.(*lua.LTable).ForEach(func(slotKey, slotVal lua.LValue) {
			slot, ok := slotKey.(lua.LNumber)
			if !ok || slotVal.Type() != lua.LTTable {
				return
			}
			page[int(slot)] = r.parseEntryTable(slotVal.(*lua.LTable))
		})
		r.pages[string(pageName)] = page
	})
}

// parseEntryTable converts a Lua key-entry table to an AppEntry.
// Must be called with the Lua VM accessible (no luaMu needed during Load;
// call only while luaMu is held during runtime updates via luaUpdateKey).
func (r *AppRunner) parseEntryTable(tbl *lua.LTable) *AppEntry {
	entry := &AppEntry{TextColor: [3]int{255, 255, 255}}

	if v := r.L.GetField(tbl, "text"); v.Type() == lua.LTString {
		entry.Text = v.String()
	}
	if v := r.L.GetField(tbl, "color"); v.Type() == lua.LTTable {
		ct := v.(*lua.LTable)
		entry.Color[0] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 1)))
		entry.Color[1] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 2)))
		entry.Color[2] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 3)))
	}
	if v := r.L.GetField(tbl, "text_color"); v.Type() == lua.LTTable {
		ct := v.(*lua.LTable)
		entry.TextColor[0] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 1)))
		entry.TextColor[1] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 2)))
		entry.TextColor[2] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 3)))
	}
	if v := r.L.GetField(tbl, "icon"); v.Type() == lua.LTString {
		raw := v.String()
		scriptDir := filepath.Dir(r.scriptPath)
		if resolved, err := resolver.ResolveString(raw, scriptDir, r.configDir, r.packages); err == nil {
			entry.Icon = resolved
		}
	}
	if v := r.L.GetField(tbl, "folder"); v.Type() == lua.LTString {
		entry.FolderPage = v.String()
	}
	if v := r.L.GetField(tbl, "action"); v.Type() == lua.LTFunction {
		entry.Action = v.(*lua.LFunction)
	}
	if v := r.L.GetField(tbl, "passive"); v.Type() == lua.LTFunction {
		entry.Passive = v.(*lua.LFunction)
	}
	return entry
}

// ── Rendering ─────────────────────────────────────────────────────────────────

// RenderKey returns the appearance for slot key on the current page.
// If app_passive is defined, calls it. Otherwise reads from the page table,
// calling per-entry passive functions for dynamic keys.
func (r *AppRunner) RenderKey(key int) *KeyAppearance {
	if r.hasAppPassive {
		return r.callAppPassive(key)
	}
	return r.renderFromPageTable(key)
}

func (r *AppRunner) callAppPassive(key int) *KeyAppearance {
	if !r.luaMu.TryLock() {
		return nil
	}
	defer r.luaMu.Unlock()

	if r.L == nil {
		return nil
	}

	fn := r.module.RawGetString("app_passive")
	if fn.Type() != lua.LTFunction {
		return nil
	}
	r.L.Push(fn)
	r.L.Push(lua.LNumber(key))
	r.L.Push(r.state)
	if err := r.L.PCall(2, 1, nil); err != nil {
		fmt.Printf("[!] app_passive(key=%d): %v\n", key, err)
		return nil
	}
	ret := r.L.Get(-1)
	r.L.Pop(1)
	if ret.Type() != lua.LTTable {
		return nil
	}
	return r.parseAppearance(ret.(*lua.LTable))
}

func (r *AppRunner) renderFromPageTable(key int) *KeyAppearance {
	r.mu.RLock()
	page := r.currentPageNoLock()
	pageEntries := r.pages[page]
	r.mu.RUnlock()

	if pageEntries == nil {
		return nil
	}
	entry := pageEntries[key]
	if entry == nil {
		return nil
	}

	// Per-entry passive function: try to call it, fall back to static fields.
	if entry.Passive != nil {
		if !r.luaMu.TryLock() {
			return r.entryToAppearance(entry)
		}
		defer r.luaMu.Unlock()
		if r.L == nil {
			return r.entryToAppearance(entry)
		}
		r.L.Push(entry.Passive)
		r.L.Push(lua.LNumber(key))
		r.L.Push(r.state)
		if err := r.L.PCall(2, 1, nil); err != nil {
			fmt.Printf("[!] app entry passive(key=%d): %v\n", key, err)
			return r.entryToAppearance(entry)
		}
		ret := r.L.Get(-1)
		r.L.Pop(1)
		if ret.Type() == lua.LTTable {
			return r.parseAppearance(ret.(*lua.LTable))
		}
		return r.entryToAppearance(entry)
	}

	return r.entryToAppearance(entry)
}

func (r *AppRunner) entryToAppearance(e *AppEntry) *KeyAppearance {
	return &KeyAppearance{
		Color:     e.Color,
		TextColor: e.TextColor,
		Text:      e.Text,
		Icon:      e.Icon,
	}
}

// parseAppearance converts a Lua appearance table to a KeyAppearance.
// Must be called with luaMu held.
func (r *AppRunner) parseAppearance(tbl *lua.LTable) *KeyAppearance {
	a := &KeyAppearance{TextColor: [3]int{255, 255, 255}}
	if v := r.L.GetField(tbl, "text"); v.Type() == lua.LTString {
		a.Text = v.String()
	}
	if v := r.L.GetField(tbl, "color"); v.Type() == lua.LTTable {
		ct := v.(*lua.LTable)
		a.Color[0] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 1)))
		a.Color[1] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 2)))
		a.Color[2] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 3)))
	}
	if v := r.L.GetField(tbl, "text_color"); v.Type() == lua.LTTable {
		ct := v.(*lua.LTable)
		a.TextColor[0] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 1)))
		a.TextColor[1] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 2)))
		a.TextColor[2] = int(lua.LVAsNumber(r.L.RawGetInt(ct, 3)))
	}
	if v := r.L.GetField(tbl, "icon"); v.Type() == lua.LTString {
		raw := v.String()
		scriptDir := filepath.Dir(r.scriptPath)
		if resolved, err := resolver.ResolveString(raw, scriptDir, r.configDir, r.packages); err == nil {
			a.Icon = resolved
		}
	}
	return a
}

// ── Input handling ────────────────────────────────────────────────────────────

// HandleKey processes a key press in app mode.
// If app_key is defined, calls it. Otherwise delegates to page-table dispatch.
func (r *AppRunner) HandleKey(key int) error {
	r.luaMu.Lock()
	defer r.luaMu.Unlock()

	if r.L == nil {
		return nil
	}

	if r.hasAppKey {
		fn := r.module.RawGetString("app_key")
		if fn.Type() != lua.LTFunction {
			return nil
		}
		r.L.Push(fn)
		r.L.Push(lua.LNumber(key))
		r.L.Push(r.state)
		if err := r.L.PCall(2, 0, nil); err != nil {
			return fmt.Errorf("app_key(key=%d): %w", key, err)
		}
		return nil
	}

	r.defaultKey(key)
	return nil
}

// ── Internal navigation (called from Lua callbacks; luaMu already held) ───────

// currentPageNoLock returns the current page name.
// Must be called with mu held (at least read-locked).
func (r *AppRunner) currentPageNoLock() string {
	if len(r.navStack) == 0 {
		return r.startPage
	}
	return r.navStack[len(r.navStack)-1]
}

// luaNavigate pushes a page onto the nav stack and signals a re-render.
// Called from Lua (luaMu held by caller's PCall context).
func (r *AppRunner) luaNavigate(page string) {
	r.mu.Lock()
	r.navStack = append(r.navStack, page)
	r.mu.Unlock()
	r.L.SetField(r.state, "_page", lua.LString(page))
	if r.onRerender != nil {
		r.onRerender()
	}
}

// luaBack pops the nav stack. If at root, calls luaExit.
// Called from Lua (luaMu held by caller's PCall context).
func (r *AppRunner) luaBack() {
	r.mu.Lock()
	if len(r.navStack) > 1 {
		r.navStack = r.navStack[:len(r.navStack)-1]
		page := r.navStack[len(r.navStack)-1]
		r.mu.Unlock()
		r.L.SetField(r.state, "_page", lua.LString(page))
		if r.onRerender != nil {
			r.onRerender()
		}
		return
	}
	r.mu.Unlock()
	r.luaExit()
}

// luaExit fires the exit callback, handing control back to the session.
// Called from Lua (luaMu held by caller's PCall context).
func (r *AppRunner) luaExit() {
	if r.onExit != nil {
		r.onExit()
	}
}

// luaUpdateKey parses an entry table and replaces the slot in the page table.
// Called from Lua (luaMu held by caller's PCall context).
func (r *AppRunner) luaUpdateKey(page string, slot int, tbl *lua.LTable) {
	entry := r.parseEntryTable(tbl)
	r.mu.Lock()
	if r.pages[page] == nil {
		r.pages[page] = make(AppPage)
	}
	r.pages[page][slot] = entry
	r.mu.Unlock()
	if r.onRerender != nil {
		r.onRerender()
	}
}

// defaultKey runs page-table action/folder dispatch for key.
// May be called from Lua context (luaMu held) or from HandleKey directly.
// Releases mu before any Lua PCall to prevent lock inversion.
func (r *AppRunner) defaultKey(key int) {
	r.mu.RLock()
	page := r.currentPageNoLock()
	pageEntries := r.pages[page]
	r.mu.RUnlock()

	if pageEntries == nil {
		return
	}
	entry := pageEntries[key]
	if entry == nil {
		return
	}

	if entry.FolderPage != "" {
		r.luaNavigate(entry.FolderPage)
		return
	}

	if entry.Action != nil {
		if r.L == nil {
			return
		}
		r.L.Push(entry.Action)
		r.L.Push(r.state)
		if err := r.L.PCall(1, 0, nil); err != nil {
			fmt.Printf("[!] app action(key=%d): %v\n", key, err)
		}
	}
}
