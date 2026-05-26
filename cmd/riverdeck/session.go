package main

import (
	"context"
	"fmt"
	"image/color"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	pkgappearance "github.com/merith-tk/riverdeck/pkg/appearance"
	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/platform"
	"github.com/merith-tk/riverdeck/pkg/scripting"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// DeviceSession holds all runtime state for a single physical (or WS) device.
// Each session has its own navigator, script manager, sleep timer, GIF
// animations, and emergency kill combo tracking.
type DeviceSession struct {
	// Identity
	deviceID      string
	configDir     string // effective session config root
	rootConfigDir string // always the global root (for packages)
	config        *Config

	// Runtime
	device    streamdeck.DeviceIface
	scriptMgr *scripting.ScriptManager
	nav       streamdeck.NavigatorIface
	ctx       context.Context
	cancel    context.CancelFunc

	// Settings overlay
	inSettings       bool
	settingsPage     int
	exitConfirming   bool

	// Display sleep / timeout
	sleepMu      sync.Mutex
	sleeping     bool
	sleepTimer   *time.Timer
	lastActivity time.Time

	// Per-key GIF animation goroutines.
	gifAnimsMu sync.Mutex
	gifAnims   map[int]context.CancelFunc

	// Emergency kill combo
	heldKeysMu sync.Mutex
	heldKeys   map[int]bool
	panicCombo []int

	backHeld bool

	// Callbacks for app-level orchestration.
	onShutdownRequest func()
	onRestartRequest  func()
}

// NewDeviceSession creates a session, boots the script manager and navigator,
// and wires key-update callbacks. The caller must start Run() in a goroutine.
// parentCtx is typically the app-level context — when parentCtx is cancelled,
// this session's event loop will also stop.
func NewDeviceSession(
	dev streamdeck.DeviceIface,
	info streamdeck.DeviceInfo,
	configDir, rootConfigDir string,
	config *Config,
	parentCtx context.Context,
	onShutdown, onRestart func(),
) (*DeviceSession, error) {
	s := &DeviceSession{
		deviceID:          info.Serial,
		configDir:         configDir,
		rootConfigDir:     rootConfigDir,
		config:            config,
		device:            dev,
		gifAnims:          make(map[int]context.CancelFunc),
		heldKeys:          make(map[int]bool),
		onShutdownRequest: onShutdown,
		onRestartRequest:  onRestart,
	}
	s.ctx, s.cancel = context.WithCancel(parentCtx)

	// Compute the emergency kill combo from device geometry.
	cols := dev.Cols()
	rows := dev.Rows()
	centerRow := rows / 2
	centerCol := cols / 2
	candidates := []int{
		0,                          // top-left
		cols - 1,                   // top-right
		centerRow*cols + centerCol, // center
		(rows - 1) * cols,          // bottom-left
		rows*cols - 1,              // bottom-right
	}
	seen := make(map[int]bool)
	for _, k := range candidates {
		if !seen[k] {
			seen[k] = true
			s.panicCombo = append(s.panicCombo, k)
		}
	}
	if len(s.panicCombo) < 3 {
		s.panicCombo = nil
		log.Printf("[*] Device %s: emergency kill combo disabled (device too small)", info.Serial)
	} else {
		log.Printf("[*] Device %s: emergency kill combo: keys %v", info.Serial, s.panicCombo)
	}

	// Set brightness
	if err := dev.SetBrightness(config.Application.Brightness); err != nil {
		log.Printf("Device %s: SetBrightness failed: %v", info.Serial, err)
	}

	// Save device geometry so the editor can show this device.
	inputs := make([]layout.CachedInput, dev.Keys())
	for i := 0; i < dev.Keys(); i++ {
		inputs[i] = layout.CachedInput{
			ID: fmt.Sprintf("key-%d", i), Type: "button",
			X: i % dev.Cols(), Y: i / dev.Cols(),
			ImageWidth: dev.PixelSize(), ImageHeight: dev.PixelSize(),
			HasImage: true, HasText: true,
		}
	}
	if err := layout.SaveDeviceGeometry(rootConfigDir, &layout.DeviceGeometry{
		ID: info.Serial, Name: info.Model.Name, Source: "hardware",
		Rows: dev.Rows(), Cols: dev.Cols(),
		Inputs: inputs, LastSeen: time.Now(),
	}); err != nil {
		log.Printf("[!] Device %s: geometry save error: %v", info.Serial, err)
	}

	// Create script manager — packages root is always the global dir.
	s.scriptMgr = scripting.NewScriptManager(dev, info.Serial, configDir, rootConfigDir, config.Application.PassiveFPS)
	if err := s.scriptMgr.Boot(s.ctx); err != nil {
		log.Printf("Device %s: script boot warning: %v", info.Serial, err)
	}
	s.scriptMgr.SetKeyUpdateCallback(s.keyUpdateCallback)

	// Create navigator
	s.nav = s.createNavigator()
	s.nav.SetScriptValidator(s.scriptMgr.IsUsableScript)

	// Start the passive update loop
	s.scriptMgr.StartPassiveLoop()

	return s, nil
}

func (s *DeviceSession) keyUpdateCallback(keyIndex int, appearance *scripting.KeyAppearance) {
	if appearance == nil {
		return
	}
	if s.inSettings {
		return
	}
	s.sleepMu.Lock()
	isSleeping := s.sleeping
	s.sleepMu.Unlock()
	if isSleeping {
		return
	}

	if appearance.Icon != "" {
		if strings.ToLower(filepath.Ext(appearance.Icon)) == ".gif" {
			s.startGIFAnim(keyIndex, appearance)
			return
		}
		s.stopGIFAnim(keyIndex)
	} else {
		s.stopGIFAnim(keyIndex)
	}

	pkgappearance.ApplyKeyAppearance(s.device, s.nav, keyIndex, appearance)
}

// ── GIF animation ────────────────────────────────────────────────────────────

func (s *DeviceSession) stopGIFAnim(keyIndex int) {
	s.gifAnimsMu.Lock()
	defer s.gifAnimsMu.Unlock()
	if cancel, ok := s.gifAnims[keyIndex]; ok {
		cancel()
		delete(s.gifAnims, keyIndex)
	}
}

func (s *DeviceSession) startGIFAnim(keyIndex int, appearance *scripting.KeyAppearance) {
	data, err := imaging.LoadGIFFrames(appearance.Icon)
	if err != nil {
		log.Printf("Device %s: GIF load failed for key %d: %v", s.deviceID, keyIndex, err)
		return
	}
	if len(data.Frames) == 0 {
		return
	}

	text := appearance.Text
	textColor := color.RGBA{
		R: uint8(appearance.TextColor[0]),
		G: uint8(appearance.TextColor[1]),
		B: uint8(appearance.TextColor[2]),
		A: 255,
	}

	preEncoded := make([][]byte, len(data.Frames))
	for i, frame := range data.Frames {
		resized := s.device.ResizeImage(frame)
		if text != "" {
			resized = s.nav.RenderTextOnImage(resized, text, textColor)
		}
		enc, err := s.device.EncodeKeyImage(keyIndex, resized)
		if err != nil {
			log.Printf("Device %s: GIF pre-encode frame %d: %v", s.deviceID, i, err)
			return
		}
		preEncoded[i] = enc
	}

	s.gifAnimsMu.Lock()
	if cancel, ok := s.gifAnims[keyIndex]; ok {
		cancel()
	}
	ctx, cancel := context.WithCancel(s.ctx)
	s.gifAnims[keyIndex] = cancel
	s.gifAnimsMu.Unlock()

	go func() {
		frameTarget := time.Now()
		for {
			for i := range data.Frames {
				select {
				case <-ctx.Done():
					return
				default:
				}

				s.sleepMu.Lock()
				isSleeping := s.sleeping
				s.sleepMu.Unlock()
				if !isSleeping && !s.inSettings {
					_ = s.device.WriteKeyData(keyIndex, preEncoded[i])
				}

				delay := 1
				if i < len(data.Delays) && data.Delays[i] > 0 {
					delay = data.Delays[i]
				}
				frameTarget = frameTarget.Add(time.Duration(delay) * 10 * time.Millisecond)
				sleepFor := time.Until(frameTarget)
				if sleepFor > 0 {
					select {
					case <-ctx.Done():
						return
					case <-time.After(sleepFor):
					}
				} else {
					select {
					case <-ctx.Done():
						return
					default:
					}
				}
			}
		}
	}()
}

func (s *DeviceSession) stopAllGIFAnims() {
	s.gifAnimsMu.Lock()
	defer s.gifAnimsMu.Unlock()
	for key, cancel := range s.gifAnims {
		cancel()
		delete(s.gifAnims, key)
	}
}

// ── Display sleep ────────────────────────────────────────────────────────────

func (s *DeviceSession) resetSleepTimer() {
	s.sleepMu.Lock()
	defer s.sleepMu.Unlock()

	if s.sleepTimer != nil {
		s.sleepTimer.Stop()
		s.sleepTimer = nil
	}

	if s.config.Application.Timeout <= 0 {
		return
	}

	duration := time.Duration(s.config.Application.Timeout) * time.Second
	s.sleepTimer = time.AfterFunc(duration, func() {
		s.sleepMu.Lock()
		defer s.sleepMu.Unlock()
		if !s.sleeping {
			s.sleeping = true
			log.Printf("[*] Device %s: display sleeping (timeout)", s.deviceID)
			_ = s.device.SetBrightness(0)
		}
	})
}

func (s *DeviceSession) wakeDisplay() bool {
	s.sleepMu.Lock()
	defer s.sleepMu.Unlock()

	if !s.sleeping {
		return false
	}
	s.sleeping = false
	log.Printf("[*] Device %s: display waking up", s.deviceID)
	_ = s.device.SetBrightness(s.config.Application.Brightness)
	return true
}

// ── Settings overlay ─────────────────────────────────────────────────────────

var timeoutValues = []int{0, 30, 60, 120, 300}

type settingsLayout struct {
	exit    int
	edit    int
	reload  int
	openDir int
	brtDown int
	brtVal  int
	brtUp   int
	tmoDown int
	tmoVal  int
	tmoUp   int
}

func (s *DeviceSession) settingsBackKey() int { return 0 }

func (s *DeviceSession) settingsScrollUpKey() int { return s.device.Cols() }

func (s *DeviceSession) settingsScrollDownKey() int { return s.device.Cols() * 2 }

func (s *DeviceSession) settingsContentKeys() []int {
	cols := s.device.Cols()
	rows := s.device.Rows()
	keys := make([]int, 0, (cols-1)*rows)
	for row := 0; row < rows; row++ {
		for col := 1; col < cols; col++ {
			keys = append(keys, row*cols+col)
		}
	}
	return keys
}

func (s *DeviceSession) settingsPageCount() int {
	if s.device.Rows() >= 3 {
		return 1
	}
	return 2
}

func (s *DeviceSession) calcSettingsLayout() settingsLayout {
	rows := s.device.Rows()
	page := s.settingsPage
	cc := s.device.Cols() - 1
	if cc < 1 {
		cc = 1
	}

	row0 := 0
	row1 := cc
	row2 := cc * 2

	sl := settingsLayout{
		exit: row0 + 0,
		edit: -1, reload: -1, openDir: -1,
		brtDown: -1, brtVal: -1, brtUp: -1,
		tmoDown: -1, tmoVal: -1, tmoUp: -1,
	}

	switch {
	case cc >= 4:
		sl.edit = row0 + 1
		sl.reload = row0 + 2
		sl.openDir = row0 + cc - 1
	case cc == 3:
		sl.reload = row0 + 1
		sl.openDir = row0 + 2
	case cc == 2:
		sl.openDir = row0 + 1
	}

	if rows >= 3 {
		sl.brtDown = row1 + 0
		sl.brtVal = row1 + 1
		sl.brtUp = row1 + 2
		sl.tmoDown = row2 + 0
		sl.tmoVal = row2 + 1
		sl.tmoUp = row2 + 2
	} else {
		if page == 0 {
			sl.brtDown = row1 + 0
			sl.brtVal = row1 + 1
			sl.brtUp = row1 + 2
		} else {
			sl.tmoDown = row1 + 0
			sl.tmoVal = row1 + 1
			sl.tmoUp = row1 + 2
		}
	}

	return sl
}

func (s *DeviceSession) enterSettings() {
	s.inSettings = true
	log.Printf("[*] Device %s: entering settings menu", s.deviceID)
	s.renderSettingsPage()
}

func (s *DeviceSession) exitSettings() {
	s.inSettings = false
	s.exitConfirming = false
	log.Printf("[*] Device %s: exiting settings menu", s.deviceID)
	if err := s.nav.RenderPage(); err != nil {
		log.Printf("Device %s: RenderPage after settings exit: %v", s.deviceID, err)
	}
	s.updateVisibleScripts()
}

func (s *DeviceSession) renderSettingsPage() {
	sl := s.calcSettingsLayout()
	contentKeys := s.settingsContentKeys()

	totalKeys := s.device.Keys()
	for i := 0; i < totalKeys; i++ {
		_ = s.device.SetKeyColor(i, color.RGBA{0, 0, 0, 255})
	}

	if s.settingsPage > 0 {
		backImg := s.nav.CreateTextImageWithColors("PG^", color.RGBA{60, 60, 60, 255}, color.White)
		_ = s.device.SetImage(s.settingsBackKey(), backImg)
	} else {
		backImg := s.nav.CreateTextImageWithColors("<-", color.RGBA{100, 100, 100, 255}, color.White)
		_ = s.device.SetImage(s.settingsBackKey(), backImg)
	}

	totalPages := s.settingsPageCount()
	t1Key := s.settingsScrollUpKey()
	t2Key := s.settingsScrollDownKey()
	if t1Key < totalKeys {
		if s.settingsPage > 0 {
			t1Img := s.nav.CreateTextImageWithColors("PG^", color.RGBA{60, 60, 60, 255}, color.White)
			_ = s.device.SetImage(t1Key, t1Img)
		} else {
			t1Img := s.nav.CreateTextImageWithColors("---", color.RGBA{20, 20, 20, 255}, color.RGBA{60, 60, 60, 255})
			_ = s.device.SetImage(t1Key, t1Img)
		}
	}
	if t2Key < totalKeys {
		if s.settingsPage < totalPages-1 {
			t2Img := s.nav.CreateTextImageWithColors("PGv", color.RGBA{60, 60, 60, 255}, color.White)
			_ = s.device.SetImage(t2Key, t2Img)
		} else {
			t2Img := s.nav.CreateTextImageWithColors("---", color.RGBA{20, 20, 20, 255}, color.RGBA{60, 60, 60, 255})
			_ = s.device.SetImage(t2Key, t2Img)
		}
	}

	setSlot := func(slot int, text string, bg, fg color.RGBA) {
		if slot < 0 || slot >= len(contentKeys) {
			return
		}
		img := s.nav.CreateTextImageWithColors(text, bg, fg)
		_ = s.device.SetImage(contentKeys[slot], img)
	}

	if s.exitConfirming {
		setSlot(sl.exit, "SURE?", color.RGBA{200, 0, 0, 255}, color.RGBA{255, 220, 220, 255})
	} else {
		setSlot(sl.exit, "EXIT", color.RGBA{140, 20, 20, 255}, color.RGBA{255, 180, 180, 255})
	}
	setSlot(sl.edit, "EDIT", color.RGBA{60, 20, 100, 255}, color.RGBA{200, 150, 255, 255})
	setSlot(sl.reload, "RELOAD", color.RGBA{20, 100, 20, 255}, color.RGBA{160, 255, 160, 255})
	setSlot(sl.openDir, "CFGDIR", color.RGBA{20, 80, 80, 255}, color.RGBA{160, 230, 230, 255})

	setSlot(sl.brtDown, "BRT-", color.RGBA{40, 40, 120, 255}, color.RGBA{160, 160, 255, 255})
	setSlot(sl.brtVal,
		fmt.Sprintf("B:%d%%", s.config.Application.Brightness),
		color.RGBA{20, 20, 60, 255}, color.RGBA{200, 200, 255, 255})
	setSlot(sl.brtUp, "BRT+", color.RGBA{40, 40, 120, 255}, color.RGBA{160, 160, 255, 255})

	setSlot(sl.tmoDown, "TMO-", color.RGBA{40, 80, 40, 255}, color.RGBA{160, 255, 160, 255})
	tmoText := fmtTimeout(s.config.Application.Timeout)
	setSlot(sl.tmoVal, tmoText, color.RGBA{20, 40, 20, 255}, color.RGBA{160, 255, 160, 255})
	setSlot(sl.tmoUp, "TMO+", color.RGBA{40, 80, 40, 255}, color.RGBA{160, 255, 160, 255})
}

func (s *DeviceSession) handleSettingsKeyEvent(keyIndex int) error {
	totalPages := s.settingsPageCount()

	if keyIndex == s.settingsBackKey() {
		if s.settingsPage > 0 {
			s.settingsPage--
			s.renderSettingsPage()
			return nil
		}
		s.exitSettings()
		return nil
	}

	if keyIndex == s.settingsScrollUpKey() {
		if s.settingsPage > 0 {
			s.settingsPage--
			s.renderSettingsPage()
		}
		return nil
	}

	if t2Key := s.settingsScrollDownKey(); t2Key < s.device.Keys() && keyIndex == t2Key {
		if s.settingsPage < totalPages-1 {
			s.settingsPage++
			s.renderSettingsPage()
		}
		return nil
	}

	contentKeys := s.settingsContentKeys()
	sl := s.calcSettingsLayout()

	slot := -1
	for i, ck := range contentKeys {
		if ck == keyIndex {
			slot = i
			break
		}
	}

	switch slot {
	case sl.exit:
	default:
		if s.exitConfirming {
			s.exitConfirming = false
			s.renderSettingsPage()
			return nil
		}
	}

	switch slot {
	case sl.exit:
		if !s.exitConfirming {
			s.exitConfirming = true
			s.renderSettingsPage()
			go func() {
				time.Sleep(3 * time.Second)
				if s.exitConfirming {
					s.exitConfirming = false
					s.renderSettingsPage()
				}
			}()
		} else {
			log.Printf("[*] Device %s: EXIT confirmed - shutting down", s.deviceID)
			if sl.exit < len(contentKeys) {
				img := s.nav.CreateTextImageWithColors("BYE",
					color.RGBA{180, 0, 0, 255},
					color.RGBA{255, 200, 200, 255})
				_ = s.device.SetImage(contentKeys[sl.exit], img)
			}
			time.Sleep(500 * time.Millisecond)
			if s.onShutdownRequest != nil {
				s.onShutdownRequest()
			}
		}
		return nil
	case sl.reload:
		log.Printf("[*] Device %s: RELOAD pressed - restarting", s.deviceID)
		if sl.reload < len(contentKeys) {
			img := s.nav.CreateTextImageWithColors("...",
				color.RGBA{80, 60, 0, 255},
				color.RGBA{255, 210, 80, 255})
			_ = s.device.SetImage(contentKeys[sl.reload], img)
		}
		time.Sleep(300 * time.Millisecond)
		if s.onRestartRequest != nil {
			s.onRestartRequest()
		}
		return nil
	case sl.openDir:
		log.Printf("[*] Device %s: opening config directory: %s", s.deviceID, s.configDir)
		if err := platform.OpenFolder(s.configDir); err != nil {
			log.Printf("Device %s: openConfigDir: %v", s.deviceID, err)
		}
		return nil
	case sl.edit:
		log.Printf("[*] Device %s: opening layout editor", s.deviceID)
		// Editor launch uses primary device (handled by App)
		return nil
	case sl.brtDown:
		s.adjustBrightness(-5)
	case sl.brtUp:
		s.adjustBrightness(+5)
	case sl.tmoDown:
		s.stepTimeout(-1)
	case sl.tmoUp:
		s.stepTimeout(+1)
	default:
		return nil
	}

	s.persistConfig()
	s.renderSettingsPage()
	return nil
}

func (s *DeviceSession) adjustBrightness(delta int) {
	v := s.config.Application.Brightness + delta
	if v < 5 {
		v = 5
	}
	if v > 100 {
		v = 100
	}
	s.config.Application.Brightness = v
	if err := s.device.SetBrightness(v); err != nil {
		log.Printf("Device %s: SetBrightness: %v", s.deviceID, err)
	}
	log.Printf("[*] Device %s: Brightness -> %d%%", s.deviceID, v)
}

func (s *DeviceSession) stepTimeout(delta int) {
	current := s.config.Application.Timeout
	idx := 0
	for i, v := range timeoutValues {
		if v == current {
			idx = i
			break
		}
	}
	idx = (idx + delta + len(timeoutValues)) % len(timeoutValues)
	s.config.Application.Timeout = timeoutValues[idx]
	log.Printf("[*] Device %s: Timeout -> %s", s.deviceID, fmtTimeout(s.config.Application.Timeout))
	s.resetSleepTimer()
}

func (s *DeviceSession) persistConfig() {
	if err := SaveConfig(s.config, s.configDir); err != nil {
		log.Printf("Device %s: SaveConfig: %v", s.deviceID, err)
	}
}

// ── Key event handling ───────────────────────────────────────────────────────

func (s *DeviceSession) handleKeyEvent(event streamdeck.KeyEvent) error {
	s.heldKeysMu.Lock()
	if event.Pressed {
		s.heldKeys[event.Key] = true
	} else {
		delete(s.heldKeys, event.Key)
	}
	panicTriggered := false
	if event.Pressed && len(s.panicCombo) > 0 {
		allHeld := true
		for _, k := range s.panicCombo {
			if !s.heldKeys[k] {
				allHeld = false
				break
			}
		}
		panicTriggered = allHeld
	}
	newBackHeld := s.heldKeys[streamdeck.KeyBack]
	backTransition := newBackHeld != s.backHeld
	if backTransition {
		s.backHeld = newBackHeld
	}
	s.heldKeysMu.Unlock()

	if backTransition {
		s.handleBackHoldChange(newBackHeld)
	}

	if panicTriggered {
		s.triggerEmergencyExit()
		return nil
	}

	if !event.Pressed {
		return nil
	}

	if newBackHeld && event.Key != streamdeck.KeyBack {
		return nil
	}

	s.lastActivity = time.Now()
	s.resetSleepTimer()

	if s.wakeDisplay() {
		if s.inSettings {
			s.renderSettingsPage()
		} else {
			_ = s.nav.RenderPage()
		}
		return nil
	}

	if s.inSettings {
		return s.handleSettingsKeyEvent(event.Key)
	}

	if event.Key == streamdeck.KeyBack && s.nav.IsAtRoot() && s.nav.PageIndex() == 0 {
		s.enterSettings()
		return nil
	}

	if event.Key == s.nav.Toggle1Key() {
		if s.nav.NextPage() {
			s.stopAllGIFAnims()
			s.scriptMgr.SetVisibleScripts(nil)
			if err := s.nav.RenderPage(); err != nil {
				log.Printf("Device %s: RenderPage failed: %v", s.deviceID, err)
			}
			s.updateVisibleScripts()
			return nil
		}
		if s.scriptMgr.HasT1Script() {
			go func() {
				if err := s.scriptMgr.TriggerT1(); err != nil {
					log.Printf("Device %s: T1 trigger: %v", s.deviceID, err)
				}
			}()
		}
		return nil
	}

	if event.Key == s.nav.Toggle2Key() {
		if s.scriptMgr.HasT2Script() {
			go func() {
				if err := s.scriptMgr.TriggerT2(); err != nil {
					log.Printf("Device %s: T2 trigger: %v", s.deviceID, err)
				}
			}()
		}
		return nil
	}

	item, navigated, err := s.nav.HandleKeyPress(event.Key)
	if err != nil {
		return fmt.Errorf("handling key press: %w", err)
	}

	if navigated {
		s.stopAllGIFAnims()
		s.scriptMgr.SetVisibleScripts(nil)
		if err := s.nav.RenderPage(); err != nil {
			log.Printf("Device %s: RenderPage failed: %v", s.deviceID, err)
		}
		s.updateVisibleScripts()

		page, _ := s.nav.LoadPage()
		if page != nil {
			relPath, _ := filepath.Rel(s.configDir, page.Path)
			if relPath == "." {
				relPath = "/"
			} else {
				relPath = "/" + relPath
			}
			log.Printf("[*] Device %s: Navigated to: %s (%d items)", s.deviceID, relPath, len(page.Items))
		}
	} else if item != nil {
		log.Printf("[*] Device %s: Action triggered: %s", s.deviceID, item.Name)
		if item.Script != "" {
			log.Printf("    Script: %s", item.Script)
			scriptPath := item.Script
			go func() {
				if err := s.scriptMgr.TriggerScript(scriptPath); err != nil {
					log.Printf("Device %s: Script error: %v", s.deviceID, err)
				}
				s.scriptMgr.RefreshScript(scriptPath)
			}()
		}
	}

	return nil
}

func (s *DeviceSession) updateVisibleScripts() {
	s.scriptMgr.SetVisibleScripts(s.nav.GetVisibleScripts())

	dirScript := s.nav.CurrentDirScript()
	t1Script, t2Script := "", ""
	if dirScript != "" {
		if runner := s.scriptMgr.GetRunner(dirScript); runner != nil {
			if runner.HasT1Passive() || runner.HasT1Trigger() {
				t1Script = dirScript
			}
			if runner.HasT2Passive() || runner.HasT2Trigger() {
				t2Script = dirScript
			}
		}
	}
	s.scriptMgr.SetToggleScripts(t1Script, s.nav.Toggle1Key(), t2Script, s.nav.Toggle2Key())
}

func (s *DeviceSession) handleBackHoldChange(held bool) {
	if held {
		log.Printf("[*] Device %s: Back key held - input lock active", s.deviceID)
	} else {
		log.Printf("[*] Device %s: Back key released - input lock cleared", s.deviceID)
	}
}

func (s *DeviceSession) triggerEmergencyExit() {
	fmt.Printf("\n[!!!] Device %s: EMERGENCY EXIT: corners+center combo detected -- killing process\n", s.deviceID)
	for i := 0; i < s.device.Keys(); i++ {
		_ = s.device.SetKeyColor(i, color.RGBA{255, 0, 0, 255})
	}
	time.Sleep(300 * time.Millisecond)
	_ = s.device.SetBrightness(0)
	_ = s.device.Clear()
	s.device.Close()
	streamdeck.Exit()
	os.Exit(1) //nolint:revive
}

// ── Editor ───────────────────────────────────────────────────────────────────

// OpenEditor launches the riverdeck-wails editor binary alongside this one.
// rootConfigPath is the global config dir (not session-specific) so the
// editor always opens the main layout file.
func (s *DeviceSession) OpenEditor(rootConfigPath string) {
	helperName := "riverdeck-wails"
	if runtime.GOOS == "windows" {
		helperName += ".exe"
	}
	exe, err := os.Executable()
	if err != nil {
		log.Printf("[!] Device %s: Could not resolve executable path: %v", s.deviceID, err)
		return
	}
	helper := filepath.Join(filepath.Dir(exe), helperName)
	if _, statErr := os.Stat(helper); statErr != nil {
		log.Printf("[!] Device %s: Editor binary not found: %s", s.deviceID, helper)
		return
	}
	cmd := exec.Command(helper,
		"-configdir", rootConfigPath,
		"-cols", fmt.Sprintf("%d", s.device.Cols()),
		"-rows", fmt.Sprintf("%d", s.device.Rows()),
		"-model", s.device.ModelName(),
	)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if startErr := cmd.Start(); startErr != nil {
		log.Printf("[!] Device %s: Failed to start editor: %v", s.deviceID, startErr)
		return
	}
	log.Printf("[*] Device %s: Editor window spawned (pid %d)", s.deviceID, cmd.Process.Pid)
}

// ── Navigator creation ──────────────────────────────────────────────────────

func (s *DeviceSession) createNavigator() streamdeck.NavigatorIface {
	style := s.config.UI.NavigationStyle

	useLayout := false
	switch style {
	case "layout":
		useLayout = true
	case "auto":
		useLayout = layout.Exists(s.configDir)
	}

	if useLayout {
		lay, err := layout.LoadForDevice(s.configDir, s.deviceID)
		if err != nil {
			log.Printf("[!] Device %s: layout load error (%v), falling back to folder navigation", s.deviceID, err)
		} else if lay != nil && len(lay.Pages) > 0 {
			log.Printf("[*] Device %s: Navigation mode: layout (%d pages)", s.deviceID, len(lay.Pages))
			nav := streamdeck.NewLayoutNavigator(s.device, s.configDir, lay)
			nav.SetPackages(s.scriptMgr.PackageInfos())
			return nav
		} else if style == "layout" {
			lay = layout.NewEmpty()
			if serr := layout.SaveLayout(s.configDir, "default", lay); serr != nil {
				log.Printf("[!] Device %s: Could not create blank layout.json: %v", s.deviceID, serr)
			} else {
				log.Printf("[*] Device %s: Navigation mode: layout (new empty layout created)", s.deviceID)
			}
			nav := streamdeck.NewLayoutNavigator(s.device, s.configDir, lay)
			nav.SetPackages(s.scriptMgr.PackageInfos())
			return nav
		}
	}

	log.Printf("[*] Device %s: Navigation mode: folder", s.deviceID)
	return streamdeck.NewNavigator(s.device, s.configDir)
}

// ── Event loop ───────────────────────────────────────────────────────────────

// Run starts this device session's event loop, blocking until the session
// context is cancelled (device disconnected / app shutdown).
func (s *DeviceSession) Run() {
	log.Printf("[*] Device %s: session starting", s.deviceID)

	s.scriptMgr.SetVisibleScripts(nil)
	if err := s.nav.RenderPage(); err != nil {
		log.Printf("Device %s: initial RenderPage error: %v", s.deviceID, err)
	}

	page, _ := s.nav.LoadPage()
	if page != nil {
		log.Printf("[*] Device %s: Current: %s (%d items, page %d/%d)",
			s.deviceID, page.Path, len(page.Items), page.PageIndex+1, page.TotalPages)
	}

	s.lastActivity = time.Now()
	s.resetSleepTimer()
	s.updateVisibleScripts()

	events := make(chan streamdeck.KeyEvent, 10)
	s.device.ListenKeys(s.ctx, events)

	for event := range events {
		if err := s.handleKeyEvent(event); err != nil {
			log.Printf("Device %s: key event error: %v", s.deviceID, err)
		}
	}

	log.Printf("[*] Device %s: session ended", s.deviceID)
}

// Shutdown cleans up this session's resources.
func (s *DeviceSession) Shutdown() {
	s.stopAllGIFAnims()
	if s.scriptMgr != nil {
		s.scriptMgr.Shutdown()
	}
	if s.device != nil {
		_ = s.device.SetBrightness(0)
		_ = s.device.Clear()
		s.device.Close()
	}
}
