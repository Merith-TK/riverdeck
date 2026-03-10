// Package scripting provides Lua script execution and lifecycle management
// for the NOMAD Stream Deck interface.
//
// # Script Architecture
//
// Every Lua script must return a table ("module") containing any combination
// of three optional functions:
//
//	local script = {}
//
//	-- Runs as a coroutine. Use system.sleep() to yield.
//	function script.background(state) ... end
//
//	-- Called at the passive FPS rate while the key is visible.
//	function script.passive(key, state) ... end
//
//	-- Called when the key is pressed.
//	function script.trigger(state) ... end
//
//	return script
//
// # State
//
// The `state` table is created once per script and passed to all three
// functions. Use it to share data across calls (e.g. cached values,
// counters, flags).
//
// # Background Workers
//
// background() runs as a gopher-lua coroutine. Call system.sleep(ms) to yield
// back to Go so that passive() and trigger() can execute. The restart policy
// (RESTART_POLICY global) controls behaviour on error or normal exit.
package scripting

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/merith-tk/riverdeck/pkg/lualib"
	"github.com/merith-tk/riverdeck/pkg/scripting/modules"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	lua "github.com/yuin/gopher-lua"
)

// RestartPolicy defines how background workers handle errors.
type RestartPolicy int

const (
	RestartAlways RestartPolicy = iota // Always restart on error (default)
	RestartNever                       // Never restart, fail permanently
	RestartOnce                        // Restart once, then stop
)

// KeyAppearance defines how a key should look (returned by passive).
type KeyAppearance struct {
	Color     [3]int // RGB color (0-255)
	Text      string // Text to display
	TextColor [3]int // Text color RGB
	Image     string // Path to image file (future)
}

// ScriptRunner manages a single Lua script's lifecycle.
type ScriptRunner struct {
	mu    sync.RWMutex
	luaMu sync.Mutex // serialises ALL operations on r.L (PCall, Resume, NewThread)

	// Script info
	ScriptPath string
	ScriptName string // Filename without .lua

	// Lua state (persistent for shared state)
	L     *lua.LState
	state *lua.LTable // Shared state table

	// Table pool for reducing allocations
	tablePool sync.Pool

	// Module table returned by script (always required)
	module *lua.LTable

	// Function availability
	hasBackground bool
	hasPassive    bool
	hasTrigger    bool

	// hasDaemon is true when the module defines a daemon() function.
	// Daemon scripts are package-level background workers managed by
	// ScriptManager; they are never shown as deck buttons.
	hasDaemon bool

	// T1 / T2 toggle-key functions (driven by .directory.lua of the current folder)
	hasT1Passive bool
	hasT1Trigger bool
	hasT2Passive bool
	hasT2Trigger bool

	// Background worker
	bgCtx      context.Context
	bgCancel   context.CancelFunc
	bgRunning  bool
	bgRestarts int
	// bgFuncName is the module-table key used by the running coroutine.
	// "background" for normal scripts, "daemon" for package daemons.
	bgFuncName    string
	restartPolicy RestartPolicy

	// Background coroutine support
	bgThread       *lua.LState // Coroutine for background function
	bgThreadCancel context.CancelFunc
	bgSleepUntil   time.Time      // When to resume from sleep
	bgFunc         *lua.LFunction // Cached background function

	// Device access
	device    *streamdeck.Device
	configDir string

	// Package library search paths (from .packages/*/lib/) and cross-script store.
	// Both are set by ScriptManager before registerModules() is called.
	packageLibPaths []string
	store           *modules.StoreModule

	// packageDataDir is the absolute path to this package's data/ directory.
	// When non-empty, the pkg_data module is preloaded and scoped to this dir.
	// Empty for normal button/directory scripts, which have no write access.
	packageDataDir string

	// Refresh callback (called when script wants display update)
	onRefresh func()

	// passiveCache holds the most recently rendered appearance per passive
	// function name ("passive", "t1_passive", "t2_passive").
	//
	// Written only while luaMu is held (successful passive run).
	// Read only when luaMu cannot be acquired (background is busy).
	// These two cases are mutually exclusive, so no additional mutex is needed.
	passiveCache map[string]*KeyAppearance
}

// NewScriptRunner creates a runner for a Lua script.
//
// packageLibPaths lists absolute paths to lib/ directories from installed
// .packages/ entries; they are prepended to Lua's package.path so that
// require('mylib') resolves to <packageDir>/lib/mylib.lua.
//
// store is the shared cross-script key-value store exposed as require('store');
// pass nil to disable the store module (e.g. for isolated test runners).
//
// packageDataDir is the absolute path to the package's data/ directory.
// When non-empty the pkg_data module is preloaded and scoped to that directory.
// Pass an empty string for normal button/directory scripts.
func NewScriptRunner(scriptPath string, dev *streamdeck.Device, configDir string, packageLibPaths []string, store *modules.StoreModule, packageDataDir string) (*ScriptRunner, error) {
	r := &ScriptRunner{
		ScriptPath:      scriptPath,
		ScriptName:      filepath.Base(scriptPath[:len(scriptPath)-4]), // Remove .lua
		device:          dev,
		configDir:       configDir,
		packageLibPaths: packageLibPaths,
		store:           store,
		packageDataDir:  packageDataDir,
		restartPolicy:   RestartAlways,
		passiveCache:    make(map[string]*KeyAppearance),
		tablePool: sync.Pool{
			New: func() interface{} {
				return &lua.LTable{}
			},
		},
	}

	// Create Lua state
	r.L = lua.NewState()

	// Create shared state table (persists across all function calls)
	r.state = r.L.NewTable()
	r.L.SetGlobal("state", r.state)

	// Register modules and set globals
	r.registerModules()

	// Load the script (defines functions or returns module)
	if err := r.L.DoFile(scriptPath); err != nil {
		r.L.Close()
		return nil, fmt.Errorf("failed to load script %s: %w", scriptPath, err)
	}

	// Script must return a module table
	result := r.L.Get(-1)
	if result.Type() != lua.LTTable {
		r.L.Close()
		return nil, fmt.Errorf("script %s must return a table (got %s)", filepath.Base(scriptPath), result.Type())
	}
	r.L.Pop(1)
	r.module = result.(*lua.LTable)

	// Detect available functions
	r.hasBackground = r.module.RawGetString("background").Type() == lua.LTFunction
	r.hasPassive = r.module.RawGetString("passive").Type() == lua.LTFunction
	r.hasTrigger = r.module.RawGetString("trigger").Type() == lua.LTFunction
	r.hasDaemon = r.module.RawGetString("daemon").Type() == lua.LTFunction
	r.hasT1Passive = r.module.RawGetString("t1_passive").Type() == lua.LTFunction
	r.hasT1Trigger = r.module.RawGetString("t1_trigger").Type() == lua.LTFunction
	r.hasT2Passive = r.module.RawGetString("t2_passive").Type() == lua.LTFunction
	r.hasT2Trigger = r.module.RawGetString("t2_trigger").Type() == lua.LTFunction

	fmt.Printf("[*] Loaded %s (bg=%v passive=%v trigger=%v t1=%v/%v t2=%v/%v)\n",
		r.ScriptName, r.hasBackground, r.hasPassive, r.hasTrigger,
		r.hasT1Passive, r.hasT1Trigger, r.hasT2Passive, r.hasT2Trigger)

	// Check for restart policy setting
	policy := r.L.GetGlobal("RESTART_POLICY")
	if policy.Type() == lua.LTString {
		switch policy.String() {
		case "never":
			r.restartPolicy = RestartNever
		case "once":
			r.restartPolicy = RestartOnce
		case "always":
			r.restartPolicy = RestartAlways
		}
	}

	return r, nil
}

// registerModules adds all available modules to the Lua state.
func (r *ScriptRunner) registerModules() {
	// Device/system modules (need runtime context)
	shellMod := modules.NewShellModule()
	httpMod := modules.NewHTTPModule()
	systemMod := modules.NewSystemModule(r.requestRefresh)
	sdMod := modules.NewStreamDeckModule(r.device)
	fileMod := modules.NewFileModule()

	r.L.PreloadModule("shell", shellMod.Loader)
	r.L.PreloadModule("http", httpMod.Loader)
	r.L.PreloadModule("system", systemMod.Loader)
	r.L.PreloadModule("streamdeck", sdMod.Loader)
	r.L.PreloadModule("file", fileMod.Loader)

	// Go-native stdlib (lualib) - zero disk I/O on require()
	lualib.RegisterUtils(r.L)
	lualib.RegisterStrings(r.L)
	lualib.RegisterJSON(r.L)
	lualib.RegisterTime(r.L)
	lualib.RegisterLog(r.L)

	// Register the shared cross-script store if one was provided.
	if r.store != nil {
		r.L.PreloadModule("store", r.store.Loader)
	}

	// Register the package-scoped data module when this script is a package script.
	// Regular button/directory scripts have packageDataDir == "" and never receive
	// this module, so they cannot write to the package data directory directly.
	if r.packageDataDir != "" {
		pkgData, err := modules.NewPackageDataModule(r.packageDataDir)
		if err != nil {
			fmt.Printf("[!] pkg_data: failed to init data dir for %s: %v\n", r.ScriptName, err)
		} else {
			r.L.PreloadModule("pkg_data", pkgData.Loader)
		}
	}

	// Extend package.path with every .packages/*/lib/ directory so that
	// require('mylib') resolves to the installed package's library file.
	if len(r.packageLibPaths) > 0 {
		pkg := r.L.GetGlobal("package")
		if pkgTable, ok := pkg.(*lua.LTable); ok {
			current := r.L.GetField(pkgTable, "path").String()
			extra := ""
			for _, libDir := range r.packageLibPaths {
				extra += filepath.Join(libDir, "?.lua") + ";"
			}
			r.L.SetField(pkgTable, "path", lua.LString(extra+current))
		}
	}

	// Set globals
	r.L.SetGlobal("SCRIPT_PATH", lua.LString(r.ScriptPath))
	r.L.SetGlobal("SCRIPT_NAME", lua.LString(r.ScriptName))
	r.L.SetGlobal("CONFIG_DIR", lua.LString(r.configDir))
}

// SetRefreshCallback sets the function called when script requests refresh.
func (r *ScriptRunner) SetRefreshCallback(cb func()) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.onRefresh = cb
}

// requestRefresh triggers a display refresh from within a script.
func (r *ScriptRunner) requestRefresh() {
	r.mu.RLock()
	cb := r.onRefresh
	r.mu.RUnlock()

	if cb != nil {
		cb()
	}
}

// getTable gets a table from the pool.
func (r *ScriptRunner) getTable() *lua.LTable {
	return r.tablePool.Get().(*lua.LTable)
}

// putTable returns a table to the pool after clearing it.
func (r *ScriptRunner) putTable(tbl *lua.LTable) {
	// Clear all keys from the table
	tbl.ForEach(func(key, value lua.LValue) {
		tbl.RawSet(key, lua.LNil)
	})
	r.tablePool.Put(tbl)
}

// HasBackground returns true if script defines background().
func (r *ScriptRunner) HasBackground() bool { return r.hasBackground }

// HasDaemon returns true if script defines daemon().
func (r *ScriptRunner) HasDaemon() bool { return r.hasDaemon }

// HasPassive returns true if script defines passive().
func (r *ScriptRunner) HasPassive() bool { return r.hasPassive }

// HasTrigger returns true if script defines trigger().
func (r *ScriptRunner) HasTrigger() bool { return r.hasTrigger }

// HasT1Passive returns true if script defines t1_passive().
func (r *ScriptRunner) HasT1Passive() bool { return r.hasT1Passive }

// HasT1Trigger returns true if script defines t1_trigger().
func (r *ScriptRunner) HasT1Trigger() bool { return r.hasT1Trigger }

// HasT2Passive returns true if script defines t2_passive().
func (r *ScriptRunner) HasT2Passive() bool { return r.hasT2Passive }

// HasT2Trigger returns true if script defines t2_trigger().
func (r *ScriptRunner) HasT2Trigger() bool { return r.hasT2Trigger }

// StartBackground starts the background() coroutine worker goroutine.
func (r *ScriptRunner) StartBackground(parentCtx context.Context) {
	if !r.hasBackground {
		return
	}

	r.mu.Lock()
	if r.bgRunning {
		r.mu.Unlock()
		return
	}

	r.bgCtx, r.bgCancel = context.WithCancel(parentCtx)
	r.bgRunning = true
	r.bgFuncName = "background"
	r.mu.Unlock()

	go r.backgroundLoop()
}

// StartDaemon starts the daemon() coroutine worker goroutine.
// Daemon scripts are package-level background workers: they run permanently,
// have full access to the shared store, and keep the store populated so that
// button scripts can read the data without doing their own polling.
//
// A daemon script returns a module table with a single "daemon" function:
//
//	local store = require('store')
//	local http  = require('http')
//	local M = {}
//
//	function M.daemon(state)
//	    while true do
//	        local ok, body = http.get('http://localhost:4455/status')
//	        if ok then
//	            store.set('obs.streaming', body.streaming)
//	        end
//	        system.sleep(2000)
//	    end
//	end
//
//	return M
func (r *ScriptRunner) StartDaemon(parentCtx context.Context) {
	if !r.hasDaemon {
		return
	}

	r.mu.Lock()
	if r.bgRunning {
		r.mu.Unlock()
		return
	}

	r.bgCtx, r.bgCancel = context.WithCancel(parentCtx)
	r.bgRunning = true
	r.bgFuncName = "daemon"
	r.mu.Unlock()

	go r.backgroundLoop()
}

// backgroundLoop runs the background function as a coroutine with restart logic.
func (r *ScriptRunner) backgroundLoop() {
	defer func() {
		r.mu.Lock()
		r.bgRunning = false
		if r.bgThreadCancel != nil {
			r.bgThreadCancel()
		}
		r.bgThread = nil
		r.bgFunc = nil
		r.mu.Unlock()
	}()

	for {
		select {
		case <-r.bgCtx.Done():
			return
		default:
		}

		// Run or resume background coroutine
		finished, sleepMs, err := r.runBackgroundCoroutine()

		if err != nil {
			fmt.Printf("[!] Background error in %s: %v\n", r.ScriptName, err)

			r.mu.Lock()
			r.bgRestarts++
			if r.bgThreadCancel != nil {
				r.bgThreadCancel()
			}
			r.bgThread = nil // Reset coroutine on error
			r.bgFunc = nil
			policy := r.restartPolicy
			restarts := r.bgRestarts
			r.mu.Unlock()

			// Check restart policy
			switch policy {
			case RestartNever:
				fmt.Printf("[!] %s: restart policy is 'never', stopping background\n", r.ScriptName)
				return
			case RestartOnce:
				if restarts > 1 {
					fmt.Printf("[!] %s: restart policy is 'once', max restarts reached\n", r.ScriptName)
					return
				}
				fmt.Printf("[*] %s: restarting background (attempt %d)\n", r.ScriptName, restarts)
			case RestartAlways:
				fmt.Printf("[*] %s: restarting background (attempt %d)\n", r.ScriptName, restarts)
			}

			// Brief delay before restart
			select {
			case <-r.bgCtx.Done():
				return
			case <-time.After(1 * time.Second):
			}
			continue
		}

		if finished {
			// Coroutine finished normally, restart it
			r.mu.Lock()
			if r.bgThreadCancel != nil {
				r.bgThreadCancel()
			}
			r.bgThread = nil
			r.bgFunc = nil
			r.mu.Unlock()

			// Brief pause before restarting
			select {
			case <-r.bgCtx.Done():
				return
			case <-time.After(100 * time.Millisecond):
			}
			continue
		}

		// Coroutine yielded (sleep) - wait WITHOUT holding mutex
		if sleepMs > 0 {
			select {
			case <-r.bgCtx.Done():
				return
			case <-time.After(time.Duration(sleepMs) * time.Millisecond):
			}
		} else {
			// No sleep specified, brief yield to allow other operations
			select {
			case <-r.bgCtx.Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
	}
}

// runBackgroundCoroutine runs or resumes the background coroutine.
// Returns: (finished bool, sleepMs int, err error)
func (r *ScriptRunner) runBackgroundCoroutine() (bool, int, error) {
	r.mu.Lock()

	// Get background function
	fnName := r.bgFuncName
	if fnName == "" {
		fnName = "background"
	}
	fn := r.module.RawGetString(fnName)

	if fn.Type() != lua.LTFunction {
		r.mu.Unlock()
		return true, 0, nil
	}
	bgFn := fn.(*lua.LFunction)

	// Create new coroutine if needed
	if r.bgThread == nil {
		r.bgThread, r.bgThreadCancel = r.L.NewThread()
		r.bgFunc = bgFn
	}

	// Prepare resume arguments
	var resumeArgs []lua.LValue
	if r.bgFunc != nil {
		// First resume - pass function and state
		resumeArgs = []lua.LValue{r.bgFunc, r.state}
		r.bgFunc = nil // Clear so subsequent resumes don't pass function again
	} else {
		// Subsequent resume - no function needed
		resumeArgs = []lua.LValue{nil}
	}

	r.mu.Unlock() // Release struct-field mutex before Lua execution

	// Acquire the Lua VM lock for the duration of Resume so that RunTrigger /
	// RunPassive cannot enter the same LState concurrently.
	r.luaMu.Lock()

	// Resume the coroutine (this may take time)
	var status lua.ResumeState
	var err error
	var values []lua.LValue

	if len(resumeArgs) > 1 {
		// First resume - pass function and state
		status, err, values = r.L.Resume(r.bgThread, resumeArgs[0].(*lua.LFunction), resumeArgs[1])
	} else {
		// Subsequent resume - no function needed
		status, err, values = r.L.Resume(r.bgThread, nil)
	}

	r.luaMu.Unlock() // Release Lua VM lock before re-acquiring struct mutex

	r.mu.Lock() // Re-acquire mutex for state updates
	defer r.mu.Unlock()

	if err != nil {
		return false, 0, err
	}

	if status == lua.ResumeOK {
		// Coroutine finished
		return true, 0, nil
	}

	// Coroutine yielded - check if sleep duration was passed
	sleepMs := 0
	if len(values) > 0 {
		if n, ok := values[0].(lua.LNumber); ok {
			sleepMs = int(n)
		}
	}

	return false, sleepMs, nil
}

// callBackground executes the background function directly (no coroutine).
func (r *ScriptRunner) callBackground() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	fn := r.module.RawGetString("background")
	if fn.Type() != lua.LTFunction {
		return nil
	}

	r.L.Push(fn)
	r.L.Push(r.state)

	if err := r.L.PCall(1, 0, nil); err != nil {
		return err
	}

	return nil
}

// StopBackground stops the background worker.
func (r *ScriptRunner) StopBackground() {
	r.mu.Lock()
	if r.bgCancel != nil {
		r.bgCancel()
	}
	r.mu.Unlock()
}

// parseAppearance parses a Lua table into a KeyAppearance.
// Must be called while r.mu (at minimum read-locked) and r.luaMu are already held.
func (r *ScriptRunner) parseAppearance(tbl *lua.LTable) *KeyAppearance {
	appearance := &KeyAppearance{}

	// Parse color: {r, g, b}
	if colorVal := r.L.GetField(tbl, "color"); colorVal.Type() == lua.LTTable {
		colorTbl := colorVal.(*lua.LTable)
		appearance.Color[0] = int(lua.LVAsNumber(r.L.RawGetInt(colorTbl, 1)))
		appearance.Color[1] = int(lua.LVAsNumber(r.L.RawGetInt(colorTbl, 2)))
		appearance.Color[2] = int(lua.LVAsNumber(r.L.RawGetInt(colorTbl, 3)))
	}

	if textVal := r.L.GetField(tbl, "text"); textVal.Type() == lua.LTString {
		appearance.Text = textVal.String()
	}

	if tcVal := r.L.GetField(tbl, "text_color"); tcVal.Type() == lua.LTTable {
		tcTbl := tcVal.(*lua.LTable)
		appearance.TextColor[0] = int(lua.LVAsNumber(r.L.RawGetInt(tcTbl, 1)))
		appearance.TextColor[1] = int(lua.LVAsNumber(r.L.RawGetInt(tcTbl, 2)))
		appearance.TextColor[2] = int(lua.LVAsNumber(r.L.RawGetInt(tcTbl, 3)))
	} else {
		appearance.TextColor = [3]int{255, 255, 255}
	}

	if imgVal := r.L.GetField(tbl, "image"); imgVal.Type() == lua.LTString {
		imgPath := imgVal.String()
		if strings.HasPrefix(imgPath, "http://") || strings.HasPrefix(imgPath, "https://") {
			appearance.Image = imgPath
		} else if !filepath.IsAbs(imgPath) {
			appearance.Image = filepath.Join(filepath.Dir(r.ScriptPath), imgPath)
		} else {
			appearance.Image = imgPath
		}
	}

	return appearance
}

// runNamedPassive calls fnName(keyIndex, state) and returns the parsed appearance.
//
// Lock strategy:
//   - TryLock on luaMu: if background holds the VM, skip Lua execution entirely
//     and return the cached appearance from the last successful run instead of
//     nil. This keeps the display showing stale-but-valid data rather than
//     blanking during a background HTTP call or long computation.
//   - On successful run: cache the result so future missed ticks can use it.
func (r *ScriptRunner) runNamedPassive(fnName string, keyIndex int) (*KeyAppearance, error) {
	if !r.luaMu.TryLock() {
		// Lua VM busy (background worker running) - return cached appearance.
		r.mu.RLock()
		cached := r.passiveCache[fnName]
		r.mu.RUnlock()
		return cached, nil
	}
	defer r.luaMu.Unlock()

	r.mu.RLock()

	fn := r.module.RawGetString(fnName)
	if fn.Type() != lua.LTFunction {
		r.mu.RUnlock()
		return nil, nil
	}

	r.L.Push(fn)
	r.L.Push(lua.LNumber(keyIndex))
	r.L.Push(r.state)

	if err := r.L.PCall(2, 1, nil); err != nil {
		r.mu.RUnlock()
		return nil, err
	}

	ret := r.L.Get(-1)
	r.L.Pop(1)

	if ret.Type() != lua.LTTable {
		r.mu.RUnlock()
		return nil, nil
	}

	appearance := r.parseAppearance(ret.(*lua.LTable))
	r.mu.RUnlock()

	// Cache the result under a write lock so future lock-miss ticks can return it.
	r.mu.Lock()
	r.passiveCache[fnName] = appearance
	r.mu.Unlock()

	return appearance, nil
}

// RunPassive calls passive(key, state) and returns appearance.
// Uses TryLock on luaMu to avoid blocking if background or trigger is using the Lua VM.
func (r *ScriptRunner) RunPassive(keyIndex int) (*KeyAppearance, error) {
	if !r.hasPassive {
		return nil, nil
	}
	return r.runNamedPassive("passive", keyIndex)
}

// RunT1Passive calls t1_passive(key, state) for the T1 toggle key.
func (r *ScriptRunner) RunT1Passive(keyIndex int) (*KeyAppearance, error) {
	if !r.hasT1Passive {
		return nil, nil
	}
	return r.runNamedPassive("t1_passive", keyIndex)
}

// RunT2Passive calls t2_passive(key, state) for the T2 toggle key.
func (r *ScriptRunner) RunT2Passive(keyIndex int) (*KeyAppearance, error) {
	if !r.hasT2Passive {
		return nil, nil
	}
	return r.runNamedPassive("t2_passive", keyIndex)
}

// runNamedTrigger calls fnName(state). Acquires luaMu.
func (r *ScriptRunner) runNamedTrigger(fnName string) error {
	r.luaMu.Lock()
	defer r.luaMu.Unlock()

	r.mu.RLock()
	defer r.mu.RUnlock()

	fn := r.module.RawGetString(fnName)
	if fn.Type() != lua.LTFunction {
		return nil
	}

	r.L.Push(fn)
	r.L.Push(r.state)

	if err := r.L.PCall(1, 0, nil); err != nil {
		return err
	}
	return nil
}

// RunTrigger calls trigger(state).
func (r *ScriptRunner) RunTrigger() error {
	if !r.hasTrigger {
		return nil
	}
	return r.runNamedTrigger("trigger")
}

// RunT1Trigger calls t1_trigger(state).
func (r *ScriptRunner) RunT1Trigger() error {
	if !r.hasT1Trigger {
		return nil
	}
	return r.runNamedTrigger("t1_trigger")
}

// RunT2Trigger calls t2_trigger(state).
func (r *ScriptRunner) RunT2Trigger() error {
	if !r.hasT2Trigger {
		return nil
	}
	return r.runNamedTrigger("t2_trigger")
}

// Close shuts down the runner and releases resources.
func (r *ScriptRunner) Close() {
	r.StopBackground()

	r.mu.Lock()
	if r.L != nil {
		r.L.Close()
		r.L = nil
	}
	r.mu.Unlock()
}
