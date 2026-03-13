// Package prober provides shared types and logic for probing Elgato Stream Deck devices.
package prober

// ProbeResult holds every piece of information extracted from a single device.
type ProbeResult struct {
	// -- HID identity ----------------------------------------------------------
	VendorID     uint16 `json:"vendor_id"`
	VendorIDHex  string `json:"vendor_id_hex"`
	ProductID    uint16 `json:"product_id"`
	ProductIDHex string `json:"product_id_hex"`
	Path         string `json:"path"`
	Manufacturer string `json:"manufacturer"`
	Product      string `json:"product"`
	Serial       string `json:"serial"`

	// -- Model (from built-in lookup table) ------------------------------------
	KnownModel  bool   `json:"known_model"`
	ModelName   string `json:"model_name"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
	Keys        int    `json:"keys"`
	PixelSize   int    `json:"pixel_size"`
	ImageFormat string `json:"image_format"`

	// -- Firmware --------------------------------------------------------------
	Firmware string `json:"firmware"`

	// -- Raw HID feature reports -----------------------------------------------
	FeatureReports []FeatureReportResult `json:"feature_reports"`

	// -- Decoded capabilities (from report 0x08) -------------------------------
	DeviceCaps *DeviceCapabilities `json:"device_caps,omitempty"`

	// -- Key-event packet analysis ---------------------------------------------
	KeyPacket *KeyPacketInfo `json:"key_packet,omitempty"`

	// -- Key events captured during listen window ------------------------------
	KeyEvents []CapturedKeyEvent `json:"key_events,omitempty"`

	// -- Raw packets that were not recognised as key events --------------------
	// These include dial, touch, LCD, and any other non-button HID input reports.
	// Captured during the CLI listen window and the GUI interaction step.
	RawPackets []CapturedRawPacket `json:"raw_packets,omitempty"`

	// -- Metadata --------------------------------------------------------------
	ProbeTime string   `json:"probe_time"`
	Errors    []string `json:"errors,omitempty"`
}

// FeatureReportResult is the outcome of a single GetFeatureReport call.
type FeatureReportResult struct {
	ID          byte   `json:"id"`
	IDHex       string `json:"id_hex"`
	Label       string `json:"label,omitempty"`       // human label for known report IDs
	Raw         string `json:"raw"`                   // full bytes as uppercase hex
	ASCII       string `json:"ascii,omitempty"`       // printable characters extracted from payload
	Decoded     string `json:"decoded,omitempty"`     // structured interpretation
	Unsupported bool   `json:"unsupported,omitempty"` // true = device has no such report ID
	Error       string `json:"error,omitempty"`       // unexpected error (not "no such report")
}

// DeviceCapabilities is decoded from feature report 0x08.
type DeviceCapabilities struct {
	KeysPerRow  int    `json:"keys_per_row"`
	IconWidth   int    `json:"icon_width_px"`
	IconHeight  int    `json:"icon_height_px"`
	PanelWidth  int    `json:"panel_width_px"`
	PanelHeight int    `json:"panel_height_px"`
	Raw         string `json:"raw"`
}

// KeyPacketInfo describes the structure of HID input packets that carry key states.
type KeyPacketInfo struct {
	PacketSizeBytes int    `json:"packet_size_bytes"`
	Format          string `json:"format"`     // "V2", "V1", or "unknown"
	HeaderHex       string `json:"header_hex"` // first few header bytes before key data
	KeyOffset       int    `json:"key_offset"`
	DetectedKeys    int    `json:"detected_keys"`
	SampleHex       string `json:"sample_hex"`
}

// CapturedKeyEvent is a timestamped key press/release seen during the listen window.
type CapturedKeyEvent struct {
	RelativeMS int64  `json:"relative_ms"` // ms since listen start
	KeyIndex   int    `json:"key_index"`
	Pressed    bool   `json:"pressed"`
	PacketHex  string `json:"packet_hex"` // raw packet that triggered this event
}

// CapturedRawPacket is an HID input packet that was not recognised as a key event.
// This captures dial rotations, touch events, LCD reports, and any other
// device-specific input whose format is not yet decoded.
type CapturedRawPacket struct {
	RelativeMS int64  `json:"relative_ms"` // ms since listen/interaction start
	Length     int    `json:"length"`      // number of bytes in the packet
	PacketHex  string `json:"packet_hex"`  // full packet as uppercase hex
}
