# Hardware Implementation Plan

This document tracks every known (or suspected) Stream Deck variant, what
makes it special, and exactly what code changes are required before it can be
supported by Riverdeck.

**Important:** All PIDs, row/column counts, and extra-input formats listed
below are best-effort estimates based on user knowledge **without hardware
verification**. Every section that touches extended inputs (dials, touch,
LCD strips) is gated on receiving a prober dump from a real device before
any implementation work begins. Until then, no code that touches those paths
should be merged.

---

## Priority 0 -- Improve the Prober First

> **Status: partially complete.** The two prober binaries have been
> consolidated into one: `cmd/riverdeck-debug-prober` now serves as both the
> GUI wizard (default) and CLI tool (`-c` flag). `cmd/riverdeck-gui-prober`
> no longer exists.

### What the prober now captures correctly

- **Feature reports** 0x00–0x0F (curated) or 0x00–0x2F (`--all-reports`):
  full hex, ASCII extract, and structured decode for known IDs. ✅
- **Key packet structure**: idle sampling detects format (`V1`/`V2`), byte
  offset, and key count from raw packet shape. Sample hex included. ✅
- **Key events with raw packet**: every state-change event includes the full
  raw packet hex (`packet_hex`) in the saved JSON. ✅
- **Dial interaction (GUI)**: `01 03 <dial_idx> <event_type> <value>` packets
  are parsed live for the Stream Deck + (0x009a). CW/CCW rotation and dial
  press are tracked in the input checklist. ✅
- **Conflict detection**: running Elgato / OpenDeck processes are flagged
  before probing. ✅

### Remaining gaps

- **Non-key packets not saved to JSON**: during the interactive GUI step,
  unknown packets (dials, touch) are shown in the status label but are **not
  written to the `ProbeResult`**. A user with an SD+ whose dials are not
  decoded would lose those raw bytes from the saved dump.
  - Fix needed: add a `RawPackets []CapturedRawPacket` field to `ProbeResult`
    and flush all unrecognised HID packets into it during both the CLI listen
    window (`CaptureKeyEvents`) and the GUI interaction step.
- **CLI listen drops non-key packets**: `CaptureKeyEvents` in
  `pkg/prober/probe.go` only fires on recognised key-state changes; dial and
  touch packets that arrive during `-listen` are silently discarded.
- **Touch inputs not modelled**: `inputs.go` has no `InputTouchPoint` kind
  and `buildInputSpec` never adds touch specs, so a Neo's touch strip cannot
  be exercised or recorded in the GUI wizard.

---

## Tier 1 -- Button-only devices

These are the simplest; most are already in `models.go`. Once missing PIDs
are confirmed via dump, support is a one-liner entry in the `Models` map.
No new interface methods needed.

### Stream Deck XL

| Field | Value |
|---|---|
| Keys | 32 |
| Layout | 4 rows x 8 cols |
| PID | 0x006c (V1), 0x0084 (V2) -- **already in models.go** |
| Extra inputs | none |

### Stream Deck MK1 / MK2 / Original V2

| Field | Value |
|---|---|
| Keys | 15 |
| Layout | 3 rows x 5 cols |
| PID | 0x0060 (Original), 0x006d (V2), 0x0080 (MK.2) -- **already in models.go** |
| Extra inputs | none |

### Stream Deck Mini

| Field | Value |
|---|---|
| Keys | 6 |
| Layout | 2 rows x 3 cols |
| PID | 0x0063 -- **already in models.go** |
| Extra inputs | none |

### Stream Deck Modules (6-key, 15-key, 32-key)

These are modular rack/desk units with the same button grids as Mini,
MK1/2, and XL respectively. They are to be treated as identical devices --
same `Cols`, `Rows`, `Keys`, `PixelSize`, and `ImageFormat`.

| Variant | Maps to | PID | Status |
|---|---|---|---|
| Module 6-key | Stream Deck Mini | **unknown -- needs dump** | no entry yet |
| Module 15-key | Stream Deck MK1/2 | **unknown -- needs dump** | no entry yet |
| Module 32-key | Stream Deck XL | **unknown -- needs dump** | no entry yet |

**Code change:** Add three new entries to the `Models` map in
`pkg/streamdeck/models.go` once PIDs are confirmed. No other changes.

---

## Tier 2 -- Pedal

### Stream Deck Pedal

| Field | Value |
|---|---|
| Keys | 3 (three physical pedals) |
| Layout | 1 row x 3 cols |
| PID | 0x0086 -- **already in models.go** |
| Extra inputs | none (no display) |

> **Note:** The current `models.go` entry already reflects 3 pedals as
> `Cols:3, Rows:1, Keys:3, PixelSize:0`. The Pedal was originally described
> as "1 button" but that appears to be a simplification -- the physical device
> has three foot pedals.  A dump is still needed to **confirm** the key-count
> and event packet format.

**Implementation plan:**

- Each pedal maps to one Lua file: `pedal1.lua`, `pedal2.lua`, `pedal3.lua`.
- Each script exposes exactly two top-level functions:

  ```lua
  function press()   ... end
  function release() ... end
  ```

- `Navigator.calculateKeyLayout()` must skip image rendering when
  `PixelSize == 0` (already safe to add now).
- No HID image writes are attempted for this device.

**Code changes (safe to draft now, finalize after dump):**

- `pkg/streamdeck/navigation.go` -- guard image calls behind `PixelSize > 0`
- `pkg/scripting/runner.go` -- teach runner to call `press()` / `release()`
  instead of the standard `on_key_press` when the device is a Pedal

---

## Tier 3 -- Devices with LCD strips and/or dials

All items in this tier require a prober dump **before full implementation**.
The dial packet format for the SD+ is **partially known** (see below).
Touch and LCD strip formats remain unknown until a dump is received.

---

### Stream Deck Neo

| Field | Value |
|---|---|
| Keys | 8 |
| Layout | 2 rows x 4 cols (buttons on top) |
| PID | 0x0090 -- **already in models.go** |
| Extra inputs | 2 touch points + 1 LCD info strip (both **below** the buttons) |

**New capability fields needed on `Model`:**

```go
HasTouchPoints  bool
TouchPoints     int   // 2 for Neo
HasStatusBar    bool
StatusBarWidth  int
StatusBarHeight int
```

**New `DeviceIface` methods:**

```go
ListenTouch(ctx context.Context, events chan<- TouchEvent)
SetStatusBarImage(img image.Image) error
```

**New event type:**

```go
type TouchEvent struct {
    Index   int  // 0 or 1 for Neo
    Pressed bool
}
```

**Lua API additions (`streamdeck` module):**

- `streamdeck.set_status_bar(img_data)` -- push image to the LCD strip
- Callbacks on scripts: `on_touch_press(index)` / `on_touch_release(index)`

**Touch script layout:**  Touch scripts live alongside button scripts in
the folder hierarchy as `touch1.lua` and `touch2.lua`.

---

### Stream Deck +

| Field | Value |
|---|---|
| Keys | 8 |
| Layout | 2 rows x 4 cols (buttons on top) |
| PID | 0x009a -- **already in models.go** |
| Extra inputs | 1 LCD status bar below buttons + 4 dials below the status bar |
| Dial packet format | **partially known**: `01 03 <dial_idx> <event_type> <value>` where `event_type=0x00` is rotation (`value` = signed delta) and `event_type=0x01` is press |

**New capability fields needed on `Model`:**

```go
HasDials        bool
Dials           int   // 4 for SD+
HasStatusBar    bool
StatusBarWidth  int
StatusBarHeight int
```

**New `DeviceIface` methods:**

```go
ListenDials(ctx context.Context, events chan<- DialEvent)
SetStatusBarImage(img image.Image) error
```

**New event type:**

```go
type DialEvent struct {
    Index   int  // 0-3 for SD+
    Delta   int  // positive = clockwise, negative = counter-clockwise
    Pressed bool // true = dial clicked down
}
```

**Lua API additions:**

- `streamdeck.set_status_bar(img_data)`
- Callbacks: `on_dial_rotate(index, delta)` / `on_dial_press(index, pressed)`

**Dial script layout:** Per-dial files alongside button scripts:
`dial1.lua` through `dial4.lua`.

---

### Stream Deck + XL  *(existence unverified)*

| Field | Value |
|---|---|
| Keys | 32 (claimed) |
| Layout | unknown -- **4x9=36 != 32, likely wrong** |
| PID | **unknown -- needs dump** |
| Extra inputs | LCD status bar + 6 dials (claimed) |

> **Flag:** The claimed 4 rows x 9 cols geometry does not add up to 32 keys.
> Either the key count, the column count, or the existence of this model is
> incorrect. Do **not** add any entry to `models.go` until a dump is received.

**Implementation plan:** Identical to Stream Deck + once geometry, PID, and
event formats are confirmed from a dump.

---

### Stream Deck Studio

| Field | Value |
|---|---|
| Keys | 32 |
| Layout | 2 rows x 16 cols |
| PID | **unknown -- needs dump** |
| Extra inputs | 2 dials with display meters, one on **each side** of the button grid + LCD status bar below buttons |

The side-mounted dial placement is a meaningful layout difference -- column 0
is no longer right for the "back" reserved key.

**Navigator changes required:**

- `calculateKeyLayout()` needs a "side dial" layout mode where no column is
  reserved; instead the physical left dial is mapped to "back/home".
- `Model` gains a `DialPlacement` field:

  ```go
  type DialPlacement int
  const (
      DialPlacementNone   DialPlacement = iota
      DialPlacementBelow                // SD+, SD+ XL
      DialPlacementSides                // Studio
  )
  ```

**Everything else** (event types, `DeviceIface` methods, Lua API) reuses the
same `DialEvent` / `ListenDials` / `on_dial_rotate` infrastructure as SD+.

---

## Tier 4 -- Future / skip for now

### Virtual Stream Deck

An on-screen overlay with clickable virtual buttons. No work planned until
all physical hardware variants are stable.

When eventually implemented, it would be a new simulator variant (similar to
`cmd/riverdeck-simulator`) rather than a change to the real HID device path.

---

## Change Checklist by File

| File | Change | Gated on |
|---|---|---|
| `pkg/streamdeck/models.go` | Add Module PIDs; add capability fields (`HasDials`, `Dials`, `HasTouchPoints`, `TouchPoints`, `HasStatusBar`, `StatusBarWidth/Height`, `DialPlacement`) | Each device's dump |
| `pkg/streamdeck/iface.go` | Add `ListenDials()`, `ListenTouch()`, `SetStatusBarImage()` | Neo + SD+ dumps |
| `pkg/streamdeck/device.go` | Implement above for real HID | Same |
| `pkg/streamdeck/simclient.go` | Simulator versions of above | Same |
| `pkg/streamdeck/navigation.go` | Skip image render when `PixelSize==0`; side-dial layout mode for Studio | Pedal confirmed; Studio dump |
| `pkg/scripting/modules/streamdeck.go` | Dial/touch callbacks, `set_status_bar` Lua API | Neo + SD+ dumps |
| `pkg/scripting/runner.go` | `press()`/`release()` call pattern for Pedal | Pedal dump |
| `cmd/riverdeck-debug-prober/` | Already captures key events with raw hex; **still needs**: persist unknown packets to JSON, record non-key packets in CLI listen mode, add Neo touch input kind | Priority 0 gap |
| `pkg/prober/probe.go` | Add `RawPackets []CapturedRawPacket` to `ProbeResult`; drain all unrecognised packets into it in `CaptureKeyEvents` and `StreamEvents` | Priority 0 gap |
| `pkg/prober/types.go` | Add `CapturedRawPacket` type (timestamp, length, hex) | Priority 0 gap |
| `testdata/` | Add reference probe dumps as they are received | Per device |
