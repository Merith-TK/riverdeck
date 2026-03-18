// Package layout defines the non-file-based (declarative) button layout
// data model used when UIConfig.NavigationStyle is "layout" or "auto".
//
// A Layout is a JSON document (layout.json) residing in the config directory.
// It describes an ordered list of Pages, each containing an explicit set of
// Buttons positioned by slot index.  Slot indices map directly to physical key
// numbers on the Stream Deck (row * cols + col), giving the user complete
// control over which key shows what, independent of filesystem organisation.
//
// JSON schema example:
//
//	{
//	  "pages": [
//	    {
//	      "name": "Main",
//	      "buttons": [
//	        {
//	          "slot":   1,
//	          "label":  "Volume Up",
//	          "script": "audio/volume_up.lua"
//	        },
//	        {
//	          "slot":    6,
//	          "label":   "Media",
//	          "action":  "page",
//	          "target_page": "Media"
//	        }
//	      ]
//	    },
//	    {
//	      "name": "Media",
//	      "buttons": [
//	        {"slot": 0, "label": "<-",     "action": "back"},
//	        {"slot": 1, "label": "Play",   "script": "media/play.lua"},
//	        {"slot": 2, "label": "Skip",   "script": "media/next.lua"}
//	      ]
//	    }
//	  ]
//	}
package layout

import "time"

// LayoutButton describes a single button in a layout page.
type LayoutButton struct {
	// Slot is the physical key index (row * cols + col, 0-based).
	Slot int `json:"slot"`

	// Label is the text displayed on the button.
	Label string `json:"label"`

	// Icon is an optional path to an image file shown as the button background.
	// Relative paths are resolved from the config directory.
	// May be a package icon reference: "vendor.pkg/icons/name.png".
	Icon string `json:"icon,omitempty"`

	// Action controls what happens when the button is pressed.
	// Recognised values:
	//   ""        / "script"  - run Script (default)
	//   "page"                - navigate to TargetPage
	//   "back"                - go to the previous page in the navigation stack
	//   "settings"            - open the settings overlay
	//   "home"                - designates this button as the SET/HOME key.
	//                           Every page must contain exactly one "home" button.
	//                           The button occupies any slot chosen by the author;
	//                           pressing it always navigates to the root page.
	Action string `json:"action,omitempty"`

	// Script is the Lua script path to execute when Action == "" or "script".
	// Relative paths are resolved from the config directory.
	Script string `json:"script,omitempty"`

	// TargetPage is the name of the page to navigate to when Action == "page".
	TargetPage string `json:"target_page,omitempty"`

	// Template is an optional reference to a package-provided button template
	// in the form "pkg://vendor.pkg/template_id".  When set, the template's
	// default Label, Icon, and Script are used as fallback values for unset
	// fields.
	Template string `json:"template,omitempty"`

	// Metadata holds arbitrary key/value pairs attached to this button, used
	// by package templates to store per-instance configuration (e.g. target
	// URL, volume level, application name). Keys and semantics are defined by
	// the package's ButtonTemplate.MetadataSchema.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// LayoutPage is a single named page of buttons.
type LayoutPage struct {
	// Name is a human-readable identifier used for navigation ("target_page").
	Name string `json:"name"`

	// Buttons is the list of explicitly positioned buttons on this page.
	// Slots not mentioned remain blank (black / inert).
	Buttons []LayoutButton `json:"buttons"`
}

// Layout is a single named layout containing an ordered list of pages.
// The first page (index 0) is shown at startup.
type Layout struct {
	Pages []LayoutPage `json:"pages"`
}

// LayoutFile is the top-level structure stored as layout.json.
// It holds named layout definitions and an optional device->layout mapping.
//
// Example:
//
//	{
//	  "layouts": {
//	    "default": {"pages": [...]},
//	    "gaming":  {"pages": [...]}
//	  },
//	  "devices": {
//	    "ABC123serial": "gaming"
//	  }
//	}
//
// Devices not listed in "devices" fall back to the "default" layout.
// Old-format files ({"pages":[...]}) are automatically promoted to
// layouts["default"] when loaded.
type LayoutFile struct {
	// Layouts holds all named layout definitions.
	Layouts map[string]*Layout `json:"layouts,omitempty"`

	// Devices maps device identifiers (serial / UUID) to layout names.
	// Omit a device to use "default".
	Devices map[string]string `json:"devices,omitempty"`

	// Pages is only present in old-format files; never written by new code.
	// Promoted to Layouts["default"] on load.
	Pages []LayoutPage `json:"pages,omitempty"`
}

// ButtonBySlot returns the button at slot s on this page, or nil if none.
func (p *LayoutPage) ButtonBySlot(s int) *LayoutButton {
	for i := range p.Buttons {
		if p.Buttons[i].Slot == s {
			return &p.Buttons[i]
		}
	}
	return nil
}

// PageByName returns the first page whose name matches (case-sensitive).
func (l *Layout) PageByName(name string) *LayoutPage {
	for i := range l.Pages {
		if l.Pages[i].Name == name {
			return &l.Pages[i]
		}
	}
	return nil
}

// PageIndexByName returns the index of the first page whose name matches,
// or -1 if not found.
func (l *Layout) PageIndexByName(name string) int {
	for i, p := range l.Pages {
		if p.Name == name {
			return i
		}
	}
	return -1
}

// CachedInput is a serialisable snapshot of one device input's capabilities.
type CachedInput struct {
	ID          string `json:"id"`
	Type        string `json:"type"` // "button" | "dial"
	X           int    `json:"x"`
	Y           int    `json:"y"`
	ImageWidth  int    `json:"imageWidth"`
	ImageHeight int    `json:"imageHeight"`
	HasImage    bool   `json:"hasImage"`
	HasText     bool   `json:"hasText"`
}

// DeviceGeometry is a cached snapshot of a device's identity and grid shape.
// It is written whenever a device connects and read by the editor API.
type DeviceGeometry struct {
	ID       string        `json:"id"`
	Name     string        `json:"name"`
	Source   string        `json:"source"` // "hardware" | "wsdevice"
	Rows     int           `json:"rows"`
	Cols     int           `json:"cols"`
	Inputs   []CachedInput `json:"inputs"`
	LastSeen time.Time     `json:"last_seen"`
}

// NewEmpty returns a Layout with a single empty page named "Main".
func NewEmpty() *Layout {
	return &Layout{
		Pages: []LayoutPage{
			{Name: "Main", Buttons: []LayoutButton{}},
		},
	}
}
