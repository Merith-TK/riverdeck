// Package scripting provides Lua script execution and management for Stream Deck integration.
//
// This package enables programmable Stream Deck functionality through Lua scripts,
// providing modules for system interaction, HTTP requests, shell commands, and
// Stream Deck control. Scripts can define background workers, passive updates,
// and trigger actions.
//
// Key components:
// - ScriptManager: Coordinates multiple script runners and passive updates
// - ScriptRunner: Manages individual Lua script lifecycle
// - Modules: Preloaded Lua modules for various system interactions
// - Image handling: Caching and loading of button images
//
// Contributors can extend functionality by:
// - Adding new Lua modules in the modules/ subdirectory
// - Implementing custom script runners
// - Extending the image loading system
// - Adding new script lifecycle hooks
package scripting

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/merith-tk/riverdeck/pkg/scripting/modules"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	lua "github.com/yuin/gopher-lua"
)

const (
	// DefaultPassiveFPS is the default rate at which passive functions are called.
	DefaultPassiveFPS = 30
)

// ScriptManager coordinates all script runners and the passive loop.
type ScriptManager struct {
	mu sync.RWMutex

	device     *streamdeck.Device
	configDir  string
	passiveFPS int

	// Installed .packages/ library paths - prepended to every runner's package.path.
	packageLibPaths []string

	// Shared cross-script key-value store - passed to every ScriptRunner.
	store *modules.StoreModule

	// daemonRunners holds runners for package daemon scripts.
	// These are distinct from content runners: they are never shown as deck
	// buttons and are always running as long as the app is alive.
	daemonRunners []*ScriptRunner

	// All loaded script runners, keyed by script path
	runners map[string]*ScriptRunner

	// Context for lifecycle management
	ctx    context.Context
	cancel context.CancelFunc

	// Passive loop
	passiveRunning bool
	visibleScripts map[string]int // script path -> key index (currently visible)

	// refreshCh is a buffered channel (capacity 1) used by scripts to request
	// an out-of-band passive update between ticker ticks.
	// requestRefresh() sends a non-blocking signal; passiveLoop drains it.
	refreshCh chan struct{}

	// lastDelivered tracks the most recently rendered appearance per deliver key.
	// deliverUpdate compares each new result against this cache; identical
	// appearances skip the image encode + USB write entirely.
	// Cleared on SetVisibleScripts (page navigation) so all keys repaint fresh.
	// Toggle keys use "<scriptPath>|t1" / "<scriptPath>|t2" as their keys.
	lastDelivered map[string]*KeyAppearance

	// Boot animation
	bootScriptPath string

	// Callback when passive wants to update a key
	onKeyUpdate func(keyIndex int, appearance *KeyAppearance)

	// T1 / T2 toggle-key scripts - set by the app on every navigation
	t1Script string
	t1Key    int
	t2Script string
	t2Key    int
}

// NewScriptManager creates a new script manager.
func NewScriptManager(dev *streamdeck.Device, configDir string, passiveFPS int) *ScriptManager {
	if passiveFPS <= 0 {
		passiveFPS = DefaultPassiveFPS
	}
	return &ScriptManager{
		device:         dev,
		configDir:      configDir,
		passiveFPS:     passiveFPS,
		runners:        make(map[string]*ScriptRunner),
		visibleScripts: make(map[string]int),
		lastDelivered:  make(map[string]*KeyAppearance),
		store:          modules.NewStoreModule(),
		refreshCh:      make(chan struct{}, 1),
	}
}

// SetKeyUpdateCallback sets the callback for passive key updates.
func (m *ScriptManager) SetKeyUpdateCallback(cb func(keyIndex int, appearance *KeyAppearance)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onKeyUpdate = cb
}

// Boot scans the config directory and loads all scripts.
// Runs boot animation if _boot.lua exists, then loads all scripts.
func (m *ScriptManager) Boot(ctx context.Context) error {
	m.mu.Lock()
	m.ctx, m.cancel = context.WithCancel(ctx)
	m.mu.Unlock()

	// Discover installed packages in .packages/ and build library search paths.
	packages, pkgErr := ScanPackages(m.configDir)
	if pkgErr != nil {
		fmt.Printf("[!] Warning: failed to scan packages: %v\n", pkgErr)
	}
	if len(packages) > 0 {
		fmt.Printf("[*] Installed packages (%d):\n", len(packages))
		installedIDs := make(map[string]bool, len(packages))
		for _, pkg := range packages {
			installedIDs[pkg.Manifest.ID] = true
		}
		for _, pkg := range packages {
			name := pkg.Manifest.Name
			if name == "" {
				name = pkg.Manifest.ID
			}
			ver := pkg.Manifest.Version
			if ver == "" {
				ver = "unknown"
			}
			line := fmt.Sprintf("    - %s v%s", name, ver)
			if pkg.Manifest.Description != "" {
				line += ": " + pkg.Manifest.Description
			}
			fmt.Println(line)
			for _, req := range pkg.Manifest.Requires {
				if !installedIDs[req] {
					fmt.Printf("    [!] %s requires %s (not installed)\n", pkg.Manifest.ID, req)
				}
			}
			if pkg.LibDir != "" {
				m.packageLibPaths = append(m.packageLibPaths, pkg.LibDir)
			}
		}

		// Boot daemon scripts now that packageLibPaths is fully populated.
		fmt.Println("[*] Starting package daemons...")
		for _, pkg := range packages {
			if pkg.DaemonScript == "" {
				continue
			}
			dRunner, dErr := NewScriptRunner(pkg.DaemonScript, m.device, m.configDir, m.packageLibPaths, m.store, pkg.DataDir)
			if dErr != nil {
				fmt.Printf("[!] Package %s: failed to load daemon: %v\n", pkg.Manifest.ID, dErr)
				continue
			}
			if !dRunner.HasDaemon() {
				fmt.Printf("[!] Package %s: daemon.lua has no daemon() function\n", pkg.Manifest.ID)
				dRunner.Close()
				continue
			}
			fmt.Printf("[*] Starting daemon: %s (%s)\n", pkg.Manifest.ID, pkg.DaemonScript)
			dRunner.StartDaemon(m.ctx)
			m.daemonRunners = append(m.daemonRunners, dRunner)
		}
	} else {
		fmt.Println("[*] No packages installed (.packages/ empty or absent)")
	}

	// Check for boot animation script - runs synchronously
	bootPath := filepath.Join(m.configDir, "_boot.lua")
	if _, err := os.Stat(bootPath); err == nil {
		m.bootScriptPath = bootPath
		// Run boot animation synchronously (blocks until complete)
		m.runBootAnimation()
	}

	// Scan for all .lua files recursively
	var scriptPaths []string
	packagesDir := filepath.Join(m.configDir, ".packages")
	err := filepath.Walk(m.configDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // Skip errors
		}
		// Skip the entire .packages/ tree - those scripts are managed
		// separately as daemon runners and Lua library files, not deck buttons.
		if info.IsDir() && filepath.Clean(path) == filepath.Clean(packagesDir) {
			return filepath.SkipDir
		}
		if info.IsDir() {
			return nil
		}
		if filepath.Ext(path) == ".lua" && filepath.Base(path) != "_boot.lua" {
			scriptPaths = append(scriptPaths, path)
		}
		return nil
	})

	if err != nil {
		return fmt.Errorf("failed to scan config directory: %w", err)
	}

	fmt.Printf("[*] Found %d scripts to load...\n", len(scriptPaths))

	// Load each script
	loaded := 0
	for _, scriptPath := range scriptPaths {
		runner, err := NewScriptRunner(scriptPath, m.device, m.configDir, m.packageLibPaths, m.store, "")
		if err != nil {
			fmt.Printf("[!] Failed to load %s: %v\n", filepath.Base(scriptPath), err)
			continue
		}

		// Set refresh callback
		runner.SetRefreshCallback(m.requestRefresh)

		m.mu.Lock()
		m.runners[scriptPath] = runner
		m.mu.Unlock()

		loaded++

		// Start background worker if defined
		if runner.HasBackground() {
			fmt.Printf("[*] Starting background worker: %s\n", runner.ScriptName)
			runner.StartBackground(m.ctx)
		}
	}

	fmt.Printf("[*] Loaded %d/%d scripts\n", loaded, len(scriptPaths))

	// Clear loading indicator
	if m.device != nil {
		m.device.Clear()
	}

	return nil
}

// runBootAnimation runs the optional _boot.lua animation script.
func (m *ScriptManager) runBootAnimation() {
	if m.bootScriptPath == "" {
		return
	}

	runner, err := NewScriptRunner(m.bootScriptPath, m.device, m.configDir, m.packageLibPaths, m.store, "")
	if err != nil {
		fmt.Printf("[!] Boot animation failed: %v\n", err)
		return
	}
	defer runner.Close()

	// Call the boot function from the module table
	if runner.module == nil {
		return
	}
	fn := runner.module.RawGetString("boot")
	if fn.Type() != lua.LTFunction {
		return
	}

	runner.L.Push(fn)
	if err := runner.L.PCall(0, 0, nil); err != nil {
		fmt.Printf("[!] Boot animation error: %v\n", err)
	}
}

// StartPassiveLoop starts the passive update loop at the configured FPS.
func (m *ScriptManager) StartPassiveLoop() {
	m.mu.Lock()
	if m.passiveRunning {
		m.mu.Unlock()
		return
	}
	m.passiveRunning = true
	m.mu.Unlock()

	go m.passiveLoop()
}

// passiveLoop runs passive functions at the configured FPS.
// It also listens on refreshCh for out-of-band update requests from scripts
// (e.g. after a background worker updates the shared store) and processes them
// immediately without waiting for the next ticker tick.
func (m *ScriptManager) passiveLoop() {
	fps := m.passiveFPS
	if fps <= 0 {
		fps = DefaultPassiveFPS
	}
	interval := time.Second / time.Duration(fps)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	runTick := func() {
		m.runPassiveUpdate()
		m.runTogglePassive()
	}

	for {
		select {
		case <-m.ctx.Done():
			m.mu.Lock()
			m.passiveRunning = false
			m.mu.Unlock()
			return
		case <-ticker.C:
			runTick()
		case <-m.refreshCh:
			// A script called system.refresh() - process immediately.
			// Drain any additional signals that stacked up while we were busy.
			runTick()
			for {
				select {
				case <-m.refreshCh:
				default:
					goto drained
				}
			}
		drained:
		}
	}
}

// deliverUpdate applies the dirty check and fires the key-update callback for
// one script. It is safe to call concurrently from multiple goroutines; the
// device's own mutex serialises HID writes.
//
// deliverKey is the map key used in lastDelivered. For normal scripts it is
// the script path; for toggle keys it is "<scriptPath>|t1" / "|t2".
func (m *ScriptManager) deliverUpdate(deliverKey string, keyIndex int, appearance *KeyAppearance) {
	m.mu.RLock()
	last := m.lastDelivered[deliverKey]
	callback := m.onKeyUpdate
	m.mu.RUnlock()

	if callback == nil || appearanceEqual(last, appearance) {
		return
	}

	m.mu.Lock()
	m.lastDelivered[deliverKey] = appearance
	m.mu.Unlock()

	callback(keyIndex, appearance)
}

// runPassiveUpdate calls passive() on all visible scripts concurrently.
// Each goroutine delivers its result to the device the moment it is ready,
// without waiting for other scripts to finish. USB writes are serialised by
// the device's own mutex, so concurrent delivery is safe.
func (m *ScriptManager) runPassiveUpdate() {
	m.mu.RLock()
	visible := make(map[string]int)
	for k, v := range m.visibleScripts {
		visible[k] = v
	}
	m.mu.RUnlock()

	if len(visible) == 0 {
		return
	}

	var wg sync.WaitGroup
	for scriptPath, keyIndex := range visible {
		wg.Add(1)
		go func(scriptPath string, keyIndex int) {
			defer wg.Done()

			m.mu.RLock()
			runner := m.runners[scriptPath]
			m.mu.RUnlock()

			if runner == nil || !runner.HasPassive() {
				return
			}

			appearance, err := runner.RunPassive(keyIndex)
			if err != nil || appearance == nil {
				return
			}

			// Deliver immediately - no batch, no wait for other scripts.
			m.deliverUpdate(scriptPath, keyIndex, appearance)
		}(scriptPath, keyIndex)
	}
	wg.Wait()
}

// appearanceEqual reports whether two KeyAppearance values are identical.
// Used by processBatchedUpdates to skip unnecessary device writes.
func appearanceEqual(a, b *KeyAppearance) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Color == b.Color &&
		a.Text == b.Text &&
		a.TextColor == b.TextColor &&
		a.Image == b.Image
}

// SetVisibleScripts updates which scripts are currently visible on the display.
// Map is scriptPath -> keyIndex
func (m *ScriptManager) SetVisibleScripts(scripts map[string]int) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.visibleScripts = make(map[string]int)
	for k, v := range scripts {
		m.visibleScripts[k] = v
	}
	// Clear dirty cache so all keys are written fresh after a page change.
	m.lastDelivered = make(map[string]*KeyAppearance)
}

// GetRunner returns the runner for a script path.
func (m *ScriptManager) GetRunner(scriptPath string) *ScriptRunner {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.runners[scriptPath]
}

// IsUsableScript returns true if the script has been loaded and defines at least
// one of background / passive / trigger. Used by the Navigator to filter the
// button list so that helper-only scripts are not shown as buttons.
func (m *ScriptManager) IsUsableScript(scriptPath string) bool {
	m.mu.RLock()
	runner := m.runners[scriptPath]
	m.mu.RUnlock()
	if runner == nil {
		return false
	}
	return runner.HasBackground() || runner.HasPassive() || runner.HasTrigger()
}

// SetToggleScripts registers the .directory.lua script (and physical key indices)
// that should drive the T1 and T2 reserved keys via t1_passive/t1_trigger etc.
// Pass an empty string for either path to fall back to default toggle behaviour.
func (m *ScriptManager) SetToggleScripts(t1Script string, t1Key int, t2Script string, t2Key int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.t1Script = t1Script
	m.t1Key = t1Key
	m.t2Script = t2Script
	m.t2Key = t2Key
}

// HasT1Script returns true when a script is driving the T1 key.
func (m *ScriptManager) HasT1Script() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.t1Script != ""
}

// HasT2Script returns true when a script is driving the T2 key.
func (m *ScriptManager) HasT2Script() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.t2Script != ""
}

// TriggerT1 calls t1_trigger on the registered T1 script, if any.
func (m *ScriptManager) TriggerT1() error {
	m.mu.RLock()
	runner := m.runners[m.t1Script]
	m.mu.RUnlock()
	if runner == nil {
		return nil
	}
	return runner.RunT1Trigger()
}

// TriggerT2 calls t2_trigger on the registered T2 script, if any.
func (m *ScriptManager) TriggerT2() error {
	m.mu.RLock()
	runner := m.runners[m.t2Script]
	m.mu.RUnlock()
	if runner == nil {
		return nil
	}
	return runner.RunT2Trigger()
}

// runTogglePassive runs t1_passive / t2_passive for the currently registered toggle scripts.
func (m *ScriptManager) runTogglePassive() {
	type toggleEntry struct {
		script   string
		key      int
		delivKey string
		isT1     bool
	}

	m.mu.RLock()
	entries := []toggleEntry{
		{m.t1Script, m.t1Key, m.t1Script + "|t1", true},
		{m.t2Script, m.t2Key, m.t2Script + "|t2", false},
	}
	m.mu.RUnlock()

	for _, e := range entries {
		if e.script == "" {
			continue
		}
		m.mu.RLock()
		runner := m.runners[e.script]
		m.mu.RUnlock()
		if runner == nil {
			continue
		}
		var ap *KeyAppearance
		var err error
		if e.isT1 {
			ap, err = runner.RunT1Passive(e.key)
		} else {
			ap, err = runner.RunT2Passive(e.key)
		}
		if err != nil || ap == nil {
			continue
		}
		m.deliverUpdate(e.delivKey, e.key, ap)
	}
}

// TriggerScript executes the trigger function for a script.
func (m *ScriptManager) TriggerScript(scriptPath string) error {
	m.mu.RLock()
	runner := m.runners[scriptPath]
	m.mu.RUnlock()

	if runner == nil {
		return fmt.Errorf("script not loaded: %s", scriptPath)
	}

	return runner.RunTrigger()
}

// RefreshScript immediately runs passive() for one script and pushes the result
// through the key-update callback. Use this after a trigger to update just the
// pressed button instead of redrawing the entire display.
func (m *ScriptManager) RefreshScript(scriptPath string) {
	m.mu.RLock()
	runner := m.runners[scriptPath]
	keyIndex, visible := m.visibleScripts[scriptPath]
	m.mu.RUnlock()

	if runner == nil || !visible || !runner.HasPassive() {
		return
	}

	appearance, err := runner.RunPassive(keyIndex)
	if err != nil || appearance == nil {
		return
	}

	// Force delivery even if appearance hasn't changed (explicit refresh after trigger).
	m.mu.Lock()
	delete(m.lastDelivered, scriptPath)
	m.mu.Unlock()

	m.deliverUpdate(scriptPath, keyIndex, appearance)
}

// requestRefresh is called when a script calls system.refresh().
// It sends a non-blocking signal on refreshCh so the passive loop wakes
// up immediately rather than waiting for the next ticker tick.
func (m *ScriptManager) requestRefresh() {
	select {
	case m.refreshCh <- struct{}{}:
	default: // already a signal pending--don't block
	}
}

// Shutdown stops all runners and cleans up.
func (m *ScriptManager) Shutdown() {
	m.mu.Lock()
	if m.cancel != nil {
		m.cancel()
	}

	// Close content runners
	for path, runner := range m.runners {
		runner.Close()
		delete(m.runners, path)
	}

	// Close package daemon runners
	for _, runner := range m.daemonRunners {
		runner.Close()
	}
	m.daemonRunners = nil

	m.mu.Unlock()

	fmt.Println("[*] Script manager shutdown complete")
}
