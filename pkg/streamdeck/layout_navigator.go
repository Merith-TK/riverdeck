package streamdeck

// layout_navigator.go - declarative (non-file-based) navigator for Riverdeck.
//
// LayoutNavigator implements NavigatorIface using a *layout.Layout instead of
// a filesystem hierarchy.  Pages are explicit lists of slotted buttons; the
// user controls every aspect of positioning, labelling, and ordering.
//
// Navigation model:
//   - The navigator maintains a stack of page indices.
//   - "page" action buttons push a new page index onto the stack.
//   - "back" action buttons (and the physical BackKey) pop the stack.
//   - IsAtRoot() is true when the stack has exactly one entry.
//
// Reserved keys (col 0) behaviour:
//   - BackKey/Toggle1Key/Toggle2Key return the same physical indices as the
//     folder Navigator so that the settings overlay works unchanged.
//   - RenderPage draws the standard SET/<-/T1/T2 labels for those keys first.
//   - If the current layout page explicitly places a button at a reserved slot,
//     that button's label/icon is drawn on top, overriding the default.

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"log"
	"sync"

	"github.com/merith-tk/riverdeck/pkg/imaging"
	"github.com/merith-tk/riverdeck/pkg/layout"
	"github.com/merith-tk/riverdeck/pkg/resolver"
)

// LayoutNavigator is the declarative navigator driven by a layout.Layout.
type LayoutNavigator struct {
	dev       DeviceIface
	configDir string
	lay       *layout.Layout

	// navStack holds the sequence of page indices visited.
	// navStack[0] is always 0 (the root page).
	navStack []int

	// contentKeys contains all physical key indices - in layout mode no keys
	// are hard-reserved by column; any slot can hold any button.
	contentKeys []int

	// packages is used by the resolver to locate pkg:// assets.
	packages []resolver.PackageInfo

	// scriptValidator is optionally applied to each Lua script path before
	// the button is shown.  nil == show everything.
	scriptValidator func(path string) bool
}

// NewLayoutNavigator creates a LayoutNavigator for the given device, config
// directory, and pre-loaded layout.  The layout must not be nil.
func NewLayoutNavigator(dev DeviceIface, configDir string, lay *layout.Layout) *LayoutNavigator {
	n := &LayoutNavigator{
		dev:       dev,
		configDir: configDir,
		lay:       lay,
		navStack:  []int{0},
	}
	n.calculateKeyLayout()
	return n
}

// SetPackages supplies the resolver package list for pkg:// URI resolution.
func (n *LayoutNavigator) SetPackages(pkgs []resolver.PackageInfo) {
	n.packages = pkgs
}

// calculateKeyLayout puts all physical keys into contentKeys.
// In layout mode, no column is hard-reserved; the home key is identified
// dynamically by scanning the current page for action:"home".
func (n *LayoutNavigator) calculateKeyLayout() {
	total := n.dev.Keys()
	n.contentKeys = make([]int, total)
	for i := 0; i < total; i++ {
		n.contentKeys[i] = i
	}
}

// currentPageIndex returns the index of the currently displayed page.
func (n *LayoutNavigator) currentPageIndex() int {
	if len(n.navStack) == 0 {
		return 0
	}
	return n.navStack[len(n.navStack)-1]
}

// currentPage returns the LayoutPage currently shown, or an empty page if the
// index is out of range.
func (n *LayoutNavigator) currentPage() *layout.LayoutPage {
	idx := n.currentPageIndex()
	if idx < 0 || idx >= len(n.lay.Pages) {
		return &layout.LayoutPage{Name: "?"}
	}
	return &n.lay.Pages[idx]
}

// ── NavigatorIface - key layout ───────────────────────────────────────────────

// homeSlot scans the current page for a button with action:"home" and returns
// its slot index.  Returns 0 as a safe fallback when no home button is found.
func (n *LayoutNavigator) homeSlot() int {
	for _, btn := range n.currentPage().Buttons {
		if btn.Action == "home" {
			return btn.Slot
		}
	}
	return 0
}

// BackKey returns the slot of the home button on the current page.
func (n *LayoutNavigator) BackKey() int {
	return n.homeSlot()
}

// Toggle1Key and Toggle2Key return out-of-range values in layout mode because
// T1/T2 are not hard-reserved - any slot can carry any button.
func (n *LayoutNavigator) Toggle1Key() int { return n.dev.Keys() }
func (n *LayoutNavigator) Toggle2Key() int { return n.dev.Keys() + 1 }

func (n *LayoutNavigator) GetContentKeys() []int {
	ks := make([]int, len(n.contentKeys))
	copy(ks, n.contentKeys)
	return ks
}

func (n *LayoutNavigator) ContentKeyCount() int {
	return len(n.contentKeys)
}

// IsReservedKey returns true only for the home button's slot.
// All other keys are freely assignable in layout mode.
func (n *LayoutNavigator) IsReservedKey(keyIndex int) bool {
	return keyIndex == n.homeSlot()
}

// ── NavigatorIface - navigation state ────────────────────────────────────────

func (n *LayoutNavigator) PageIndex() int { return 0 }

func (n *LayoutNavigator) IsAtRoot() bool { return len(n.navStack) <= 1 }

func (n *LayoutNavigator) CurrentPath() string {
	return n.currentPage().Name
}

// CurrentDirScript always returns "" - layout mode has no .directory.lua.
func (n *LayoutNavigator) CurrentDirScript() string { return "" }

// ── NavigatorIface - page operations ─────────────────────────────────────────

// LoadPage synthesises a Page struct from the current layout page.
// Items are included for all buttons that carry a Script (for the
// visible-scripts feed to the script manager).
func (n *LayoutNavigator) LoadPage() (*Page, error) {
	if len(n.lay.Pages) == 0 {
		return &Page{Path: "layout:Main", TotalPages: 1}, nil
	}
	lp := n.currentPage()
	var items []PageItem
	for i := range lp.Buttons {
		btn := &lp.Buttons[i]
		script, err := n.resolveScript(btn)
		if err != nil || script == "" {
			continue
		}
		if n.scriptValidator != nil && !n.scriptValidator(script) {
			continue
		}
		items = append(items, PageItem{
			Name:   btn.Label,
			Path:   "layout:" + lp.Name + "#" + fmt.Sprintf("%d", btn.Slot),
			Script: script,
		})
	}
	return &Page{
		Path:       "layout:" + lp.Name,
		Items:      items,
		ParentPath: "",
		PageIndex:  0,
		TotalPages: 1,
	}, nil
}

// RenderPage renders the current layout page to the Stream Deck.
func (n *LayoutNavigator) RenderPage() error {
	lp := n.currentPage()
	totalKeys := n.dev.Keys()

	// Build one image per physical key (nil == black).
	images := make([]image.Image, totalKeys)

	// ── Layout buttons (all slots, including the home button) ─────────────
	for i := range lp.Buttons {
		btn := &lp.Buttons[i]
		if btn.Slot < 0 || btn.Slot >= totalKeys {
			continue
		}
		images[btn.Slot] = n.renderButton(btn)
	}

	// ── Encode all keys concurrently, write serially ───────────────────────
	type frame struct {
		index int
		data  []byte
		err   error
	}
	frames := make([]frame, totalKeys)
	blackImg := func() image.Image {
		sz := n.dev.PixelSize()
		img := image.NewRGBA(image.Rect(0, 0, sz, sz))
		draw.Draw(img, img.Bounds(), image.Black, image.Point{}, draw.Src)
		return img
	}()

	var wg sync.WaitGroup
	wg.Add(totalKeys)
	for i := 0; i < totalKeys; i++ {
		i := i
		go func() {
			defer wg.Done()
			img := images[i]
			if img == nil {
				img = blackImg
			}
			data, err := n.dev.EncodeKeyImage(i, img)
			frames[i] = frame{index: i, data: data, err: err}
		}()
	}
	wg.Wait()

	for _, f := range frames {
		if f.err != nil {
			return fmt.Errorf("encode key %d: %w", f.index, f.err)
		}
		if err := n.dev.WriteKeyData(f.index, f.data); err != nil {
			return fmt.Errorf("write key %d: %w", f.index, err)
		}
	}

	// Send label text for each button that has one (no-op for hardware/sim).
	for i := range lp.Buttons {
		btn := &lp.Buttons[i]
		if btn.Slot >= 0 && btn.Slot < totalKeys && btn.Label != "" {
			_ = n.dev.SetLabel(btn.Slot, btn.Label)
		}
	}

	return nil
}

// NextPage has no effect in layout mode (no pagination within a page).
func (n *LayoutNavigator) NextPage() bool { return false }

// PrevPage has no effect in layout mode.
func (n *LayoutNavigator) PrevPage() bool { return false }

// NavigateBack pops the navigation stack, returning true if we moved up.
func (n *LayoutNavigator) NavigateBack() bool {
	if len(n.navStack) <= 1 {
		return false
	}
	n.navStack = n.navStack[:len(n.navStack)-1]
	return true
}

// NavigateToRoot resets the navigation stack to the root page.
func (n *LayoutNavigator) NavigateToRoot() {
	n.navStack = []int{0}
}

// HandleKeyPress processes a content key press.
// "page" action buttons push the target page; "back" action buttons pop; script
// buttons return a PageItem for the caller to execute.
func (n *LayoutNavigator) HandleKeyPress(keyIndex int) (*PageItem, bool, error) {
	// The home button (action:"home") always navigates to root.
	// We handle it here before loading the page button so it works even if
	// the button's Slot matches keyIndex before the switch below.
	if keyIndex == n.BackKey() {
		if !n.IsAtRoot() {
			n.NavigateToRoot()
			return nil, true, nil
		}
		// Already at root - nothing to do; the button will be rendered via renderButton.
		return nil, false, nil
	}

	lp := n.currentPage()
	btn := lp.ButtonBySlot(keyIndex)
	if btn == nil {
		return nil, false, nil // empty slot
	}

	switch btn.Action {
	case "home":
		// Home always navigates to root.
		n.NavigateToRoot()
		return nil, true, nil

	case "back":
		if n.NavigateBack() {
			return nil, true, nil
		}
		return nil, false, nil

	case "settings":
		// Caller (App) recognises the "settings" PageItem path.
		return &PageItem{Name: "Settings", Path: "settings:", Script: ""}, false, nil

	case "page":
		if btn.TargetPage == "" {
			return nil, false, nil
		}
		idx := n.lay.PageIndexByName(btn.TargetPage)
		if idx < 0 {
			return nil, false, fmt.Errorf("layout: target page %q not found", btn.TargetPage)
		}
		n.navStack = append(n.navStack, idx)
		return nil, true, nil

	default: // "" or "script"
		script, err := n.resolveScript(btn)
		if err != nil {
			log.Printf("[!] layout button slot %d: %v", btn.Slot, err)
			return nil, false, nil
		}
		if script == "" {
			return nil, false, nil
		}
		return &PageItem{
			Name:   btn.Label,
			Path:   "layout:" + btn.Script,
			Script: script,
		}, false, nil
	}
}

// ── NavigatorIface - script integration ──────────────────────────────────────

func (n *LayoutNavigator) SetScriptValidator(fn func(path string) bool) {
	n.scriptValidator = fn
}

// GetVisibleScripts returns a map of resolved script path -> physical key index
// for all script buttons on the current page.
func (n *LayoutNavigator) GetVisibleScripts() map[string]int {
	result := make(map[string]int)
	lp := n.currentPage()
	for i := range lp.Buttons {
		btn := &lp.Buttons[i]
		script, err := n.resolveScript(btn)
		if err != nil || script == "" {
			continue
		}
		if n.scriptValidator != nil && !n.scriptValidator(script) {
			continue
		}
		result[script] = btn.Slot
	}
	return result
}

// ── NavigatorIface - image helpers ───────────────────────────────────────────

// CreateTextImageWithColors delegates to Navigator's implementation via a
// shared helper so we don't duplicate that code.
func (n *LayoutNavigator) CreateTextImageWithColors(text string, bgColor, textColor color.Color) image.Image {
	return createTextImageWithColors(n.dev.PixelSize(), text, bgColor, textColor)
}

func (n *LayoutNavigator) createTextImage(text string, bgColor color.Color) image.Image {
	return n.CreateTextImageWithColors(text, bgColor, color.White)
}

// RenderTextOnImage composites centred text over an existing image.
func (n *LayoutNavigator) RenderTextOnImage(base image.Image, text string, textColor color.Color) image.Image {
	return renderTextOnImage(n.dev.PixelSize(), base, text, textColor)
}

// ── Private helpers ───────────────────────────────────────────────────────────

// resolveScript returns the absolute path of a button's Lua script, or "" if
// the button carries no script.  Uses the resolver package to support pkg://
// URIs as well as plain relative/absolute paths.
// Returns an error only when the reference is explicitly forbidden (web Lua).
func (n *LayoutNavigator) resolveScript(btn *layout.LayoutButton) (string, error) {
	if btn.Action != "" && btn.Action != "script" {
		return "", nil
	}
	if btn.Script == "" {
		return "", nil
	}
	ref := resolver.Parse(btn.Script)
	if resolver.IsLuaForbidden(ref) {
		return "", fmt.Errorf("web Lua scripts are forbidden: %s", btn.Script)
	}
	return resolver.Resolve(ref, n.configDir, n.packages)
}

// renderButton builds the image for a single layout button.
func (n *LayoutNavigator) renderButton(btn *layout.LayoutButton) image.Image {
	label := btn.Label

	// Try to load an icon image via the resolver (supports pkg:// URIs).
	if btn.Icon != "" {
		ref := resolver.Parse(btn.Icon)
		if iconPath, err := resolver.Resolve(ref, n.configDir, n.packages); err == nil {
			if img, err := imaging.LoadImage(iconPath); err == nil {
				resized := n.dev.ResizeImage(img)
				if label != "" {
					return renderTextOnImage(n.dev.PixelSize(), resized, label, color.White)
				}
				return resized
			}
		}
	}

	// Determine background colour by action type.
	var bg color.RGBA
	switch btn.Action {
	case "home":
		bg = color.RGBA{120, 80, 0, 255} // amber (same as settings key)
	case "page":
		bg = color.RGBA{30, 80, 180, 255} // blue
	case "back":
		bg = color.RGBA{100, 100, 100, 255} // grey
	case "settings":
		bg = color.RGBA{80, 20, 120, 255} // purple
	default:
		bg = color.RGBA{30, 130, 80, 255} // green (script)
	}

	if label == "" {
		switch btn.Action {
		case "home":
			label = "SET"
		case "back":
			label = "<-"
		case "settings":
			label = "MENU"
		default:
			label = "?"
		}
	}
	return n.CreateTextImageWithColors(truncateName(label, 8), bg, color.White)
}
