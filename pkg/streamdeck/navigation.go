package streamdeck

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
	"sort"
	"sync"

	"golang.org/x/image/font"
	"golang.org/x/image/font/basicfont"
	"golang.org/x/image/math/fixed"
)

// PageItem represents an item on a page (folder or action).
type PageItem struct {
	Name     string // Display name
	Path     string // Full path to the item
	IsFolder bool   // True if this is a folder
	Script   string // Path to lua script (if action)
}

// Page represents a single page of items on the Stream Deck.
type Page struct {
	Path       string     // Current directory path
	Items      []PageItem // Items on this page
	ParentPath string     // Path to parent directory (empty if root)
	PageIndex  int        // Current page index (for pagination)
	TotalPages int        // Total number of pages
}

// Reserved key indices (column 0 on a 5-column deck)
// Layout: key index = row * cols + col
// Row 0: 0,1,2,3,4
// Row 1: 5,6,7,8,9
// Row 2: 10,11,12,13,14
//
// TODO: Reserved keys are currently hardcoded for MK.2 (5 cols x 3 rows).
// These should be dynamically calculated based on the device model's row count
// and column layout. Consider: ReservedKeys = [0, cols, cols*2, ...] for col 0.
const (
	KeyBack    = 0  // Row 0, Col 0 - Navigate back
	KeyToggle1 = 5  // Row 1, Col 0 - Reserved toggle (placeholder)
	KeyToggle2 = 10 // Row 2, Col 0 - Reserved toggle (placeholder)
)

// Navigator manages folder-based navigation on a Stream Deck.
type Navigator struct {
	dev          *Device
	rootPath     string
	currentDir   string
	pageIndex    int
	contentKeys  []int // Key indices available for content (excludes column 0)
	reservedKeys []int // Key indices for reserved functions (column 0)

	// scriptValidator is called for each .lua file; if set and returns false the
	// file is hidden from the page (e.g. scripts with no recognised functions).
	scriptValidator func(path string) bool
}

// NewNavigator creates a new navigator for the given device and root config path.
func NewNavigator(dev *Device, rootPath string) *Navigator {
	n := &Navigator{
		dev:        dev,
		rootPath:   rootPath,
		currentDir: rootPath,
		pageIndex:  0,
	}
	n.calculateKeyLayout()
	return n
}

// calculateKeyLayout determines which keys are for content vs reserved.
func (n *Navigator) calculateKeyLayout() {
	cols := n.dev.Cols()
	rows := n.dev.Rows()

	n.contentKeys = nil
	n.reservedKeys = nil

	for row := 0; row < rows; row++ {
		for col := 0; col < cols; col++ {
			keyIndex := row*cols + col
			if col == 0 {
				// Column 0 is reserved
				n.reservedKeys = append(n.reservedKeys, keyIndex)
			} else {
				n.contentKeys = append(n.contentKeys, keyIndex)
			}
		}
	}
}

// GetContentKeys returns the key indices available for page content.
func (n *Navigator) GetContentKeys() []int {
	keys := make([]int, len(n.contentKeys))
	copy(keys, n.contentKeys)
	return keys
}

// ContentKeyCount returns the number of keys available for content.
func (n *Navigator) ContentKeyCount() int {
	return len(n.contentKeys)
}

// CurrentPath returns the current directory path.
func (n *Navigator) CurrentPath() string {
	return n.currentDir
}

// SetScriptValidator sets a function that is called for each .lua candidate.
// Return true to show the file, false to hide it. Useful for filtering out
// scripts that do not define any of background/passive/trigger.
func (n *Navigator) SetScriptValidator(fn func(path string) bool) {
	n.scriptValidator = fn
}

// IsAtRoot returns true if we're at the root config directory.
func (n *Navigator) IsAtRoot() bool {
	return n.currentDir == n.rootPath
}

// CurrentDirScript returns the path to the .directory.lua inside the current
// folder, or an empty string if no such file exists.
func (n *Navigator) CurrentDirScript() string {
	p := filepath.Join(n.currentDir, ".directory.lua")
	if _, err := os.Stat(p); err == nil {
		return p
	}
	return ""
}

// LoadPage loads the current page and returns page info.
func (n *Navigator) LoadPage() (*Page, error) {
	entries, err := os.ReadDir(n.currentDir)
	if err != nil {
		return nil, fmt.Errorf("read dir %s: %w", n.currentDir, err)
	}

	// Filter and sort entries
	var items []PageItem
	for _, entry := range entries {
		name := entry.Name()

		// Skip underscore-prefixed entries (internal / private)
		if len(name) > 0 && name[0] == '_' {
			continue
		}

		// .directory.lua is a special per-folder passive script, not a button
		if name == ".directory.lua" {
			continue
		}

		// All other dot-files / dot-dirs are also hidden
		if len(name) > 0 && name[0] == '.' {
			continue
		}

		if entry.IsDir() {
			item := PageItem{
				Name:     name,
				Path:     filepath.Join(n.currentDir, name),
				IsFolder: true,
			}
			// If the folder contains a .directory.lua, attach it so the
			// passive loop can drive the button's appearance.
			dirScript := filepath.Join(item.Path, ".directory.lua")
			if _, err := os.Stat(dirScript); err == nil {
				item.Script = dirScript
			}
			items = append(items, item)
			continue
		}

		// Only .lua files beyond this point
		if filepath.Ext(name) != ".lua" {
			continue
		}

		scriptPath := filepath.Join(n.currentDir, name)

		// If a validator is registered, skip scripts it rejects
		if n.scriptValidator != nil && !n.scriptValidator(scriptPath) {
			continue
		}

		items = append(items, PageItem{
			Name:   name[:len(name)-4], // strip .lua
			Path:   scriptPath,
			Script: scriptPath,
		})
	}

	// Sort: folders first, then alphabetically
	sort.Slice(items, func(i, j int) bool {
		if items[i].IsFolder != items[j].IsFolder {
			return items[i].IsFolder
		}
		return items[i].Name < items[j].Name
	})

	// Calculate pagination using content keys only (excludes reserved column)
	keysAvailable := n.ContentKeyCount()

	totalPages := 1
	if len(items) > keysAvailable {
		totalPages = (len(items) + keysAvailable - 1) / keysAvailable
	}

	// Clamp page index
	if n.pageIndex >= totalPages {
		n.pageIndex = totalPages - 1
	}
	if n.pageIndex < 0 {
		n.pageIndex = 0
	}

	// Get items for current page
	start := n.pageIndex * keysAvailable
	end := start + keysAvailable
	if end > len(items) {
		end = len(items)
	}

	pageItems := items[start:end]

	// Determine parent path
	parentPath := ""
	if !n.IsAtRoot() {
		parentPath = filepath.Dir(n.currentDir)
	}

	return &Page{
		Path:       n.currentDir,
		Items:      pageItems,
		ParentPath: parentPath,
		PageIndex:  n.pageIndex,
		TotalPages: totalPages,
	}, nil
}

// NavigateInto enters a subdirectory.
func (n *Navigator) NavigateInto(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("not a directory: %s", path)
	}
	n.currentDir = path
	n.pageIndex = 0
	return nil
}

// NavigateBack goes to the parent directory.
func (n *Navigator) NavigateBack() bool {
	if n.IsAtRoot() {
		return false
	}
	n.currentDir = filepath.Dir(n.currentDir)
	n.pageIndex = 0
	return true
}

// NavigateToRoot returns to the root config directory.
func (n *Navigator) NavigateToRoot() {
	n.currentDir = n.rootPath
	n.pageIndex = 0
}

// NextPage moves to the next page.
func (n *Navigator) NextPage() bool {
	page, err := n.LoadPage()
	if err != nil {
		return false
	}
	if n.pageIndex < page.TotalPages-1 {
		n.pageIndex++
		return true
	}
	return false
}

// PrevPage moves to the previous page.
func (n *Navigator) PrevPage() bool {
	if n.pageIndex > 0 {
		n.pageIndex--
		return true
	}
	return false
}

// RenderPage renders the current page to the Stream Deck.
// Images are encoded concurrently, then written to the device serially.
// No Clear() pass is needed — every key is explicitly overwritten.
func (n *Navigator) RenderPage() error {
	page, err := n.LoadPage()
	if err != nil {
		return err
	}

	totalKeys := n.dev.Model.Keys
	type keyFrame struct {
		index int
		data  []byte
		err   error
	}

	frames := make([]keyFrame, totalKeys)
	for i := range frames {
		frames[i].index = i
	}

	// Build image for every key (nil = black / unused)
	images := make([]image.Image, totalKeys)

	// Reserved column
	if !n.IsAtRoot() {
		images[KeyBack] = n.createTextImage("<-", color.RGBA{100, 100, 100, 255})
	} else {
		// At root the back key doubles as the settings entry point
		images[KeyBack] = n.CreateTextImageWithColors("SET", color.RGBA{120, 80, 0, 255}, color.RGBA{255, 200, 50, 255})
	}
	// T1 / T2: render a dim default; passive scripts from .directory.lua
	// will paint over these via the key-update callback.
	images[KeyToggle1] = n.createTextImage("T1", color.RGBA{30, 30, 30, 255})
	images[KeyToggle2] = n.createTextImage("T2", color.RGBA{30, 30, 30, 255})

	// Content keys
	for i, item := range page.Items {
		if i >= len(n.contentKeys) {
			break
		}
		if item.IsFolder {
			images[n.contentKeys[i]] = n.createTextImage(truncateName(item.Name, 8), color.RGBA{30, 80, 180, 255})
		} else {
			images[n.contentKeys[i]] = n.createTextImage(truncateName(item.Name, 8), color.RGBA{30, 130, 80, 255})
		}
	}
	// Any remaining content keys (no item) stay nil → black

	// Encode all keys concurrently
	blackImg := func() image.Image {
		size := n.dev.PixelSize()
		img := image.NewRGBA(image.Rect(0, 0, size, size))
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
			data, err := n.dev.EncodeKeyImage(img)
			frames[i].data = data
			frames[i].err = err
		}()
	}
	wg.Wait()

	// Write serially (HID is not goroutine-safe for concurrent writes)
	for _, f := range frames {
		if f.err != nil {
			return fmt.Errorf("encode key %d: %w", f.index, f.err)
		}
		if err := n.dev.WriteKeyData(f.index, f.data); err != nil {
			return fmt.Errorf("write key %d: %w", f.index, err)
		}
	}

	return nil
}

// renderReservedKeys renders the reserved column buttons (column 0).
func (n *Navigator) renderReservedKeys() {
	// Key 0 (row 0, col 0): Back button / settings entry at root
	if !n.IsAtRoot() {
		img := n.createTextImage("<-", color.RGBA{100, 100, 100, 255})
		n.dev.SetImage(KeyBack, img)
	} else {
		// At root - the key opens the settings menu
		img := n.CreateTextImageWithColors("SET", color.RGBA{120, 80, 0, 255}, color.RGBA{255, 200, 50, 255})
		n.dev.SetImage(KeyBack, img)
	}

	// T1 / T2: render a dim default; passive scripts from .directory.lua
	// will paint over these via the key-update callback.
	n.dev.SetImage(KeyToggle1, n.createTextImage("T1", color.RGBA{30, 30, 30, 255}))
	n.dev.SetImage(KeyToggle2, n.createTextImage("T2", color.RGBA{30, 30, 30, 255}))
}

// HandleKeyPress handles a key press and returns the action to take.
// Returns: (item *PageItem, navigated bool, err error)
// If navigated is true, the page changed. If item is non-nil, it's an action to execute.
func (n *Navigator) HandleKeyPress(keyIndex int) (*PageItem, bool, error) {
	page, err := n.LoadPage()
	if err != nil {
		return nil, false, err
	}

	// Check if this is a reserved key (column 0)
	switch keyIndex {
	case KeyBack:
		if n.NavigateBack() {
			return nil, true, nil
		}
		return nil, false, nil

	case KeyToggle1, KeyToggle2:
		// Reserved - handled upstream before HandleKeyPress is called.
		return nil, false, nil
	}

	// Check if this is a content key
	for i, ck := range n.contentKeys {
		if ck == keyIndex {
			if i < len(page.Items) {
				item := &page.Items[i]
				if item.IsFolder {
					if err := n.NavigateInto(item.Path); err != nil {
						return nil, false, err
					}
					return nil, true, nil
				}
				// It's an action/script
				return item, false, nil
			}
			return nil, false, nil // Empty key
		}
	}

	return nil, false, nil
}

// GetVisibleScripts returns a map of script paths to key indices for visible scripts.
// Includes both action scripts and folder .directory.lua passive scripts.
func (n *Navigator) GetVisibleScripts() map[string]int {
	result := make(map[string]int)

	page, err := n.LoadPage()
	if err != nil {
		return result
	}

	for i, item := range page.Items {
		if i >= len(n.contentKeys) {
			break
		}
		if item.Script != "" {
			result[item.Script] = n.contentKeys[i]
		}
	}

	return result
}

// createTextImage creates a simple image with text.
func (n *Navigator) createTextImage(text string, bgColor color.Color) image.Image {
	return n.CreateTextImageWithColors(text, bgColor, color.White)
}

// CreateTextImageWithColors creates an image with text and custom colors.
// This is exported for use by script passive updates.
func (n *Navigator) CreateTextImageWithColors(text string, bgColor, textColor color.Color) image.Image {
	size := n.dev.PixelSize()
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Fill background
	draw.Draw(img, img.Bounds(), &image.Uniform{bgColor}, image.Point{}, draw.Src)

	// Draw text centered
	d := &font.Drawer{
		Dst:  img,
		Src:  image.NewUniform(textColor),
		Face: basicfont.Face7x13,
	}

	// Calculate text position (roughly centered)
	textWidth := len(text) * 7 // basicfont is ~7px wide per char
	x := (size - textWidth) / 2
	if x < 2 {
		x = 2
	}
	y := size/2 + 4 // Center vertically

	d.Dot = fixed.Point26_6{X: fixed.I(x), Y: fixed.I(y)}
	d.DrawString(text)

	return img
}

// truncateName truncates a name to fit on a button.
func truncateName(name string, maxLen int) string {
	if len(name) <= maxLen {
		return name
	}
	return name[:maxLen-1] + "."
}
