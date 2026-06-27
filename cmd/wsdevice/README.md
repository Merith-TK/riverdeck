# WebSocket Client Devices

WebSocket client devices connect to the Riverdeck WebSocket server, declare their grid size and input layout, and receive image/label updates in return. This lets anything that can open a WebSocket act as a Riverdeck device without needing a full native driver.

The protocol is intentionally minimal: the client just declares how many rows and columns it has and what inputs it provides. How the client physically renders images and responds to label updates is entirely its own concern.

Examples of what can be built with this:
- A mobile app used as a Riverdeck device
- A web app used as a Riverdeck device
- A Raspberry Pi Zero W with GPIO buttons wired up, connecting to a host running Riverdeck over the network
- A microcontroller with a small display, forwarding button presses and rendering frames
- A Stream Deck plugged into a Pi Zero W that proxies it to Riverdeck over Wi-Fi
- A bridge to hardware not yet supported natively by Riverdeck

> **Note:** The `web/` and `android/` subdirectories in this folder are placeholders for future reference client implementations. They are currently empty.

Websocket client devices only support **layout mode** -- folder mode is not supported.

---

## Connection Lifecycle

### 1. Handshake (Client -> RiverDeck)

Immediately upon connecting, the client MUST send a `hello` message declaring its identity and capabilities. This is the minimum required payload:

```json
{
  "type": "hello",
  "id": "unique-device-id",
  "name": "My Custom Device",
  "rows": 2,
  "cols": 4,
  "inputs": [
    {
      "id": "btn0",
      "type": "button",
      "x": 0,
      "y": 0,
      "display": {
        "image": true,
        "imageWidth": 72,
        "imageHeight": 72,
        "text": true
      }
    },
    {
      "id": "btn1",
      "type": "button",
      "x": 1,
      "y": 0,
      "display": {
        "image": false,
        "text": false
      }
    },
    {
      "id": "dial0",
      "type": "dial",
      "x": 2,
      "y": 0,
      "display": {
        "image": true,
        "imageWidth": 72,
        "imageHeight": 72,
        "text": true
      }
    }
  ]
}
```

**Fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `type` | ✅ | Always `"hello"` |
| `id` | ✅ | Stable unique identifier for this device. Used to match saved layout config. Should persist across reconnects. |
| `name` | ✅ | Human-readable display name shown in the RiverDeck UI |
| `rows` | ✅ | Number of rows in the grid (used for layout editor) |
| `cols` | ✅ | Number of columns in the grid (used for layout editor) |
| `inputs` | ✅ | Array of input descriptors (see below) |

**Input descriptor fields:**

| Field | Required | Description |
|-------|----------|-------------|
| `id` | ✅ | Stable unique identifier for this input within the device |
| `type` | ✅ | One of: `"button"`, `"dial"` |
| `x` | ✅ | Column position (0-indexed). Used by the layout editor for accurate visual placement. |
| `y` | ✅ | Row position (0-indexed). Used by the layout editor for accurate visual placement. |
| `display.image` | ✅ | Whether this input can display image/icon data |
| `display.imageWidth` | if image=true | Width in pixels RiverDeck should render at |
| `display.imageHeight` | if image=true | Height in pixels RiverDeck should render at |
| `display.text` | ✅ | Whether this input can display label text |

> **Note:** If XY positions are omitted or duplicated, RiverDeck will fall back to placing inputs in declaration order (left-to-right, top-to-bottom). Providing accurate XY coordinates is strongly recommended so the layout editor reflects the physical device correctly.

---

### 2. RiverDeck Response

RiverDeck will reply with an `ack` message:

```json
{
  "type": "ack",
  "status": "ok"
}
```

If something is wrong with the handshake, RiverDeck will reply with:

```json
{
  "type": "ack",
  "status": "error",
  "reason": "missing required field: id"
}
```

After `ack`, RiverDeck will begin pushing the current layout state to the device (see **RiverDeck -> Client messages** below).

---

### 3. Layout Config Auto-Creation

If RiverDeck has no saved layout config for the connecting device's `id`, it will **automatically create a blank layout** matching the declared grid dimensions and input list. The user can then configure it in the layout editor as they would any other device.

---

### 4. Disconnect Behaviour

When a websocket client device disconnects:
- RiverDeck retains the device's layout config in memory and on disk (same as any multidevice config).
- The device is marked as **offline** in the UI.
- RiverDeck does **not** queue any outbound data for the device while it is offline. When the device reconnects and completes the handshake, RiverDeck will push a fresh full state.
- Nothing prevents third-party addons from implementing their own queuing on top of this -- but the core RiverDeck behaviour is fire-and-forget, no queuing.

---

## Messages: Client -> RiverDeck

### Button Events

```json
{
  "type": "input",
  "id": "btn0",
  "event": "press"
}
```

```json
{
  "type": "input",
  "id": "btn0",
  "event": "release"
}
```

```json
{
  "type": "input",
  "id": "btn0",
  "event": "held"
}
```

### Dial Events

```json
{
  "type": "input",
  "id": "dial0",
  "event": "press"
}
```

```json
{
  "type": "input",
  "id": "dial0",
  "event": "release"
}
```

```json
{
  "type": "input",
  "id": "dial0",
  "event": "held"
}
```

```json
{
  "type": "input",
  "id": "dial0",
  "event": "valueInc"
}
```

```json
{
  "type": "input",
  "id": "dial0",
  "event": "valueDec"
}
```

Dials also support absolute value reporting:

```json
{
  "type": "input",
  "id": "dial0",
  "event": "value",
  "value": 42,
  "valueMin": 0,
  "valueMax": 100
}
```

**Valid `event` values by input type:**

| Event | button | dial |
|-------|--------|------|
| `press` | ✅ | ✅ |
| `release` | ✅ | ✅ |
| `held` | ✅ | ✅ |
| `valueInc` | ❌ | ✅ |
| `valueDec` | ❌ | ✅ |
| `value` | ❌ | ✅ |

---

## Messages: RiverDeck -> Client

RiverDeck uses the same frame-diffing logic it uses for real hardware devices: it renders a frame per input, compares it to the last-sent frame for that input, and only pushes an update if the frame changed.

### Image/Icon Frame

Sent when an input's rendered image frame changes. Only sent to inputs that declared `display.image: true`.

```json
{
  "type": "frame",
  "id": "btn0",
  "width": 72,
  "height": 72,
  "data": "<base64-encoded image bytes>"
}
```

The image data is raw pixel bytes (format TBD by implementation -- recommend RGBA or JPEG). Resolution matches the `imageWidth`/`imageHeight` the client declared for this input.

### Label Text

Sent when an input's label text changes. Only sent to inputs that declared `display.text: true`.

```json
{
  "type": "label",
  "id": "btn0",
  "text": "Volume"
}
```

### Layout Change Notification

Sent when the active layout changes (e.g. user switches profiles).

```json
{
  "type": "layoutChange",
  "layoutId": "abc123",
  "layoutName": "Gaming"
}
```

After a `layoutChange`, RiverDeck will push fresh `frame` and `label` updates for all inputs as needed.

---

## Implementation Notes for Claude Code

- The device `id` is the stable key used to look up saved config. Treat it like a serial number -- it must survive process restarts on the client side.
- The existing multidevice config persistence path should be reused/extended to support websocket client devices. The device type should be tagged (e.g. `"source": "websocket"`) so it can be distinguished from real hardware.
- The websocket server should handle multiple simultaneous websocket client devices.
- If two clients connect with the same `id`, the behaviour should be: reject the second connection with an error, or disconnect the first and accept the second (preference: reject second, log a warning).
- Layout mode only -- if a websocket client device somehow ends up in a folder-mode context, it should be treated as an error and the device should be put in an error/offline state.
- The layout editor should use the XY coordinates from the `hello` message to render the device's physical layout accurately, including non-rectangular or sparse layouts (e.g. a Raspberry Pi with GPIO buttons not arranged in a perfect grid).