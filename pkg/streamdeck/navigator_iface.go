package streamdeck

import (
	"image"
	"image/color"
)

// NavigatorIface is satisfied by both the file-browser Navigator and the
// declarative LayoutNavigator.  The App struct holds a NavigatorIface so that
// navigation mode can be switched at start-up (folder / layout / auto) without
// changing any of the event-handling code in app.go, settings.go, or gif.go.
type NavigatorIface interface {
	// ── Key layout ────────────────────────────────────────────────────────────

	// BackKey returns the physical key index for the back/settings button.
	BackKey() int

	// Toggle1Key returns the physical key index for the T1 reserved button.
	Toggle1Key() int

	// Toggle2Key returns the physical key index for the T2 reserved button.
	Toggle2Key() int

	// GetContentKeys returns the key indices available for page content.
	GetContentKeys() []int

	// ContentKeyCount returns the number of content key slots.
	ContentKeyCount() int

	// IsReservedKey reports whether keyIndex belongs to the reserved column.
	IsReservedKey(keyIndex int) bool

	// ── Navigation state ─────────────────────────────────────────────────────

	// PageIndex returns the current page index (0-based).
	PageIndex() int

	// IsAtRoot reports whether the navigator is at the top-level page/folder.
	IsAtRoot() bool

	// CurrentPath returns a human-readable path/name for the current location.
	// For the file-browser navigator this is the directory path; for the layout
	// navigator it is the page name.
	CurrentPath() string

	// CurrentDirScript returns the path to a passive .directory.lua for the
	// current location, or empty string if none.  Layout navigator always
	// returns "".
	CurrentDirScript() string

	// ── Page operations ───────────────────────────────────────────────────────

	// LoadPage returns the current page's metadata without rendering.
	LoadPage() (*Page, error)

	// RenderPage renders the current page to the Stream Deck.
	RenderPage() error

	// NextPage advances to the next page if one exists, returning true on success.
	NextPage() bool

	// PrevPage moves to the previous page if possible, returning true on success.
	PrevPage() bool

	// NavigateBack goes one level up (parent directory or previous page in stack).
	// Returns false if already at the root/top.
	NavigateBack() bool

	// NavigateToRoot returns to the root/first page.
	NavigateToRoot()

	// HandleKeyPress processes a content key press and returns the triggered
	// item (if any) and whether a page navigation occurred.
	HandleKeyPress(keyIndex int) (*PageItem, bool, error)

	// ── Script integration ────────────────────────────────────────────────────

	// SetScriptValidator registers a function called for each Lua script path.
	// Return false to hide the script from the page.
	SetScriptValidator(fn func(path string) bool)

	// GetVisibleScripts returns a map of script path -> physical key index for
	// all scripts currently visible on the page.
	GetVisibleScripts() map[string]int

	// ── Image helpers ─────────────────────────────────────────────────────────

	// CreateTextImageWithColors builds a solid-colour image with centred text.
	CreateTextImageWithColors(text string, bgColor, textColor color.Color) image.Image

	// RenderTextOnImage composites centred text over an existing image.
	RenderTextOnImage(base image.Image, text string, textColor color.Color) image.Image
}
