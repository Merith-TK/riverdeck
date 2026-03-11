package modules

import (
	"image/color"

	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	lua "github.com/yuin/gopher-lua"
)

// StreamDeckModule exposes Stream Deck hardware control to Lua scripts.
type StreamDeckModule struct {
	device streamdeck.DeviceIface
}

// NewStreamDeckModule creates a new StreamDeck module bound to a device.
func NewStreamDeckModule(device streamdeck.DeviceIface) *StreamDeckModule {
	return &StreamDeckModule{device: device}
}

// Loader returns the Lua module loader function.
func (m *StreamDeckModule) Loader(L *lua.LState) int {
	mod := L.SetFuncs(L.NewTable(), map[string]lua.LGFunction{
		"set_color":      m.sdSetColor,
		"set_brightness": m.sdSetBrightness,
		"clear":          m.sdClear,
		"clear_key":      m.sdClearKey,
		"reset":          m.sdReset,
		"get_model":      m.sdGetModel,
		"get_keys":       m.sdGetKeys,
		"get_layout":     m.sdGetLayout,
	})
	L.Push(mod)
	return 1
}

func (m *StreamDeckModule) checkDevice(L *lua.LState) bool {
	if m.device == nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString("no device connected"))
		return false
	}
	return true
}

// sdSetColor sets a single key to a solid RGB color.
// Lua: streamdeck.set_color(key, r, g, b) -> ok, err
func (m *StreamDeckModule) sdSetColor(L *lua.LState) int {
	if !m.checkDevice(L) {
		return 2
	}
	key := L.CheckInt(1)
	r := L.CheckInt(2)
	g := L.CheckInt(3)
	b := L.CheckInt(4)
	c := color.RGBA{R: uint8(r), G: uint8(g), B: uint8(b), A: 255}
	if err := m.device.SetKeyColor(key, c); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// sdSetBrightness sets the global brightness (0-100).
// Lua: streamdeck.set_brightness(percent) -> ok, err
func (m *StreamDeckModule) sdSetBrightness(L *lua.LState) int {
	if !m.checkDevice(L) {
		return 2
	}
	if err := m.device.SetBrightness(L.CheckInt(1)); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// sdClear clears all keys to black.
// Lua: streamdeck.clear() -> ok, err
func (m *StreamDeckModule) sdClear(L *lua.LState) int {
	if !m.checkDevice(L) {
		return 2
	}
	if err := m.device.Clear(); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// sdClearKey sets a single key to black.
// Lua: streamdeck.clear_key(key) -> ok, err
func (m *StreamDeckModule) sdClearKey(L *lua.LState) int {
	if !m.checkDevice(L) {
		return 2
	}
	key := L.CheckInt(1)
	if err := m.device.SetKeyColor(key, color.RGBA{A: 255}); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// sdReset resets the deck to its factory default state.
// Lua: streamdeck.reset() -> ok, err
func (m *StreamDeckModule) sdReset(L *lua.LState) int {
	if !m.checkDevice(L) {
		return 2
	}
	if err := m.device.Reset(); err != nil {
		L.Push(lua.LFalse)
		L.Push(lua.LString(err.Error()))
		return 2
	}
	L.Push(lua.LTrue)
	L.Push(lua.LNil)
	return 2
}

// sdGetModel returns the device model name.
// Lua: streamdeck.get_model() -> string
func (m *StreamDeckModule) sdGetModel(L *lua.LState) int {
	if m.device == nil {
		L.Push(lua.LNil)
		return 1
	}
	L.Push(lua.LString(m.device.ModelName()))
	return 1
}

// sdGetKeys returns the total number of keys on the device.
// Lua: streamdeck.get_keys() -> number
func (m *StreamDeckModule) sdGetKeys(L *lua.LState) int {
	if m.device == nil {
		L.Push(lua.LNumber(0))
		return 1
	}
	L.Push(lua.LNumber(m.device.Keys()))
	return 1
}

// sdGetLayout returns the column and row counts of the key grid.
// Lua: streamdeck.get_layout() -> cols, rows
func (m *StreamDeckModule) sdGetLayout(L *lua.LState) int {
	if m.device == nil {
		L.Push(lua.LNumber(0))
		L.Push(lua.LNumber(0))
		return 2
	}
	L.Push(lua.LNumber(m.device.Cols()))
	L.Push(lua.LNumber(m.device.Rows()))
	return 2
}
