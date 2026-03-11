package main

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/sstallion/go-hid"
)

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

// curated list of feature report IDs to always probe (firmware, serial, brightness...)
var curatedReportIDs = []byte{
	0x00, 0x01, 0x02, 0x03, 0x04,
	0x05, // firmware version (V2/MK.2)
	0x06, 0x07, 0x08,
	0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f,
}

// ProbeDevice opens the device and extracts as much information as possible.
func ProbeDevice(raw hid.DeviceInfo, listenDur time.Duration, allReports bool) ProbeResult {
	r := ProbeResult{
		VendorID:     raw.VendorID,
		VendorIDHex:  fmt.Sprintf("0x%04X", raw.VendorID),
		ProductID:    raw.ProductID,
		ProductIDHex: fmt.Sprintf("0x%04X", raw.ProductID),
		Path:         raw.Path,
		Manufacturer: raw.MfrStr,
		Product:      raw.ProductStr,
		Serial:       raw.SerialNbr,
		ProbeTime:    time.Now().UTC().Format(time.RFC3339),
	}

	// -- Model lookup ----------------------------------------------------------
	model, known := streamdeck.LookupModel(raw.ProductID)
	r.KnownModel = known
	if known {
		r.ModelName = model.Name
		r.Cols = model.Cols
		r.Rows = model.Rows
		r.Keys = model.Keys
		r.PixelSize = model.PixelSize
		r.ImageFormat = model.ImageFormat
	} else {
		r.ModelName = fmt.Sprintf("Unknown Elgato device (PID 0x%04X)", raw.ProductID)
	}

	// -- Open HID device -------------------------------------------------------
	dev, err := hid.OpenPath(raw.Path)
	if err != nil {
		r.Errors = append(r.Errors, fmt.Sprintf("open: %v", err))
		return r
	}
	defer dev.Close()

	// -- Feature reports -------------------------------------------------------
	reportIDs := curatedReportIDs
	if allReports {
		reportIDs = make([]byte, 0x30)
		for i := range reportIDs {
			reportIDs[i] = byte(i)
		}
	}

	for _, id := range reportIDs {
		buf := make([]byte, 64)
		buf[0] = id
		n, err := dev.GetFeatureReport(buf)
		res := FeatureReportResult{
			ID:    id,
			IDHex: fmt.Sprintf("0x%02X", id),
			Label: knownReportLabel(id),
		}
		if err != nil {
			// "parameter is incorrect" (Windows error 0x57) means the report ID
			// simply does not exist on this device -- not a real error.
			if isUnsupportedReportErr(err) {
				res.Unsupported = true
			} else {
				res.Error = err.Error()
			}
		} else {
			data := buf[:n]
			res.Raw = strings.ToUpper(hex.EncodeToString(data))
			res.ASCII = extractASCII(data)
			res.Decoded = decodeReport(id, data)
		}
		r.FeatureReports = append(r.FeatureReports, res)

		// Extract firmware from 0x05 if not yet set
		if id == 0x05 && err == nil && r.Firmware == "" {
			r.Firmware = extractFirmware(buf[:n])
		}
		// Decode capabilities from 0x08
		if id == 0x08 && err == nil && n >= 11 {
			r.DeviceCaps = decodeCapabilities(buf[:n])
		}
	}

	// -- Key packet analysis (idle sampling) -----------------------------------
	kp := probeKeyPacket(dev)
	if kp != nil {
		r.KeyPacket = kp
		// Prefer detected key count over model if they differ and model is unknown
		if !known && kp.DetectedKeys > 0 {
			r.Keys = kp.DetectedKeys
		}
	}

	// -- Live key-event capture ------------------------------------------------
	if listenDur > 0 {
		r.KeyEvents = captureKeyEvents(dev, r.Keys, listenDur)
	}

	return r
}

// probeKeyPacket reads up to 5 input packets at idle and analyses the byte layout.
func probeKeyPacket(dev *hid.Device) *KeyPacketInfo {
	// Read a few packets using a generous buffer; most devices send 512-byte reports.
	const bufSize = 512
	const attempts = 5

	var first []byte
	for i := 0; i < attempts; i++ {
		buf := make([]byte, bufSize)
		n, err := dev.ReadWithTimeout(buf, 150*time.Millisecond)
		if err != nil || n == 0 {
			continue
		}
		first = buf[:n]
		break
	}
	if len(first) == 0 {
		return nil
	}

	kp := &KeyPacketInfo{
		PacketSizeBytes: len(first),
		SampleHex:       strings.ToUpper(hex.EncodeToString(first)),
	}

	kp.Format, kp.KeyOffset, kp.DetectedKeys = detectKeyFormat(first)
	if kp.KeyOffset > 0 && kp.KeyOffset <= len(first) {
		kp.HeaderHex = strings.ToUpper(hex.EncodeToString(first[:kp.KeyOffset]))
	}

	return kp
}

// detectKeyFormat examines one raw HID input packet and returns the format name,
// key-data byte offset, and number of keys.
//
// Known formats:
//
//	"V2" - 01 00 NN 00 [keys...]  (MK.2 / V2 / XL)  offset=4, NN=key count
//	"V1" - 01 [keys...]              (Original / Mini)  offset=1, scan for key count
func detectKeyFormat(pkt []byte) (format string, keyOffset int, keyCount int) {
	if len(pkt) < 2 || pkt[0] != 0x01 {
		return "unknown", 0, 0
	}

	// V2: 01 00 NN 00 [keys...]
	// NN is the key count embedded in the header, must be plausible (1-64).
	if len(pkt) >= 5 && pkt[1] == 0x00 && pkt[3] == 0x00 {
		nn := int(pkt[2])
		if nn >= 1 && nn <= 64 {
			return "V2", 4, nn
		}
	}

	// V1: 01 [keys...]
	// Count consecutive bytes that are 0x00 or 0x01 starting at offset 1.
	count := 0
	for _, b := range pkt[1:] {
		if b > 0x01 {
			break
		}
		count++
	}
	if count >= 1 {
		return "V1", 1, count
	}

	return "unknown", 0, 0
}

// captureKeyEvents listens for HID input reports for the given duration and
// records every key state change as a CapturedKeyEvent.
func captureKeyEvents(dev *hid.Device, expectedKeys int, dur time.Duration) []CapturedKeyEvent {
	if expectedKeys <= 0 {
		expectedKeys = 32 // generous fallback
	}

	var events []CapturedKeyEvent
	prevState := make([]bool, expectedKeys)
	start := time.Now()
	deadline := start.Add(dur)

	buf := make([]byte, 512)
	keyOffset := -1 // -1 = not yet detected

	for time.Now().Before(deadline) {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			break
		}
		timeout := remaining
		if timeout > 100*time.Millisecond {
			timeout = 100 * time.Millisecond
		}

		n, err := dev.ReadWithTimeout(buf, timeout)
		if err != nil || n == 0 {
			continue
		}
		pktHex := strings.ToUpper(hex.EncodeToString(buf[:n]))

		// Detect format from the first packet we actually receive.
		if keyOffset == -1 {
			_, keyOffset, _ = detectKeyFormat(buf[:n])
			if keyOffset == 0 {
				keyOffset = 4 // safe default
			}
		}

		for i := 0; i < expectedKeys && keyOffset+i < n; i++ {
			pressed := buf[keyOffset+i] != 0
			if pressed != prevState[i] {
				events = append(events, CapturedKeyEvent{
					RelativeMS: time.Since(start).Milliseconds(),
					KeyIndex:   i,
					Pressed:    pressed,
					PacketHex:  pktHex,
				})
				prevState[i] = pressed
			}
		}
	}

	return events
}

// extractFirmware parses the firmware string from a raw feature report 0x05.
func extractFirmware(data []byte) string {
	// Firmware bytes start at offset 6 for V2/MK.2 devices.
	const offset = 6
	if len(data) <= offset {
		return ""
	}
	for i := offset; i < len(data); i++ {
		if data[i] == 0 {
			return string(data[offset:i])
		}
	}
	return string(data[offset:])
}

// isUnsupportedReportErr returns true when the error is simply "report ID not
// supported by this device" -- on Windows this surfaces as error 0x57
// "The parameter is incorrect".
func isUnsupportedReportErr(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "0x00000057") ||
		strings.Contains(s, "parameter is incorrect") ||
		strings.Contains(s, "invalid parameter")
}

// knownReportLabel returns a human-readable label for known Elgato report IDs.
func knownReportLabel(id byte) string {
	switch id {
	case 0x03:
		return "Device state"
	case 0x04:
		return "Bootloader version"
	case 0x05:
		return "Firmware version"
	case 0x06:
		return "Serial number"
	case 0x07:
		return "Secondary firmware version"
	case 0x08:
		return "Hardware capabilities"
	case 0x09:
		return "LCD / image config"
	case 0x0a:
		return "Unknown (0x0A)"
	case 0x0b:
		return "Image chunk size"
	case 0x0c:
		return "Unknown (0x0C)"
	}
	return ""
}

// extractASCII returns a string containing only printable ASCII characters
// found in the payload (bytes 1+ to skip the report ID byte).
func extractASCII(data []byte) string {
	if len(data) < 2 {
		return ""
	}
	var sb strings.Builder
	for _, b := range data[1:] {
		if b >= 0x20 && b < 0x7f {
			sb.WriteByte(b)
		}
	}
	s := strings.TrimSpace(sb.String())
	// Only return if there are meaningful printable chars (at least 3)
	if len(s) < 3 {
		return ""
	}
	return s
}

// decodeReport attempts to produce a human-readable interpretation of a
// feature report's payload for known report IDs.
func decodeReport(id byte, data []byte) string {
	switch id {
	case 0x03:
		if len(data) >= 2 {
			return fmt.Sprintf("state=0x%02X", data[1])
		}
	case 0x04, 0x07:
		// Format: ID, length, [N-byte prefix/checksum], version-string
		// Scan for the first printable ASCII run of length >= 5
		for start := 2; start < len(data)-4; start++ {
			if data[start] >= 0x30 && data[start] <= 0x39 || // digit
				(data[start] >= 'A' && data[start] <= 'Z') ||
				(data[start] >= 'a' && data[start] <= 'z') {
				// collect version string until null or non-printable
				var sb strings.Builder
				for _, b := range data[start:] {
					if b == 0 || b < 0x20 || b >= 0x7f {
						break
					}
					sb.WriteByte(b)
				}
				if s := sb.String(); len(s) >= 5 {
					label := "bootloader_version"
					if id == 0x07 {
						label = "secondary_firmware_version"
					}
					return fmt.Sprintf("%s=%q (checksum_prefix=%s)",
						label, s,
						strings.ToUpper(hex.EncodeToString(data[2:start])))
				}
			}
		}
	case 0x05:
		v := extractFirmware(data)
		if v != "" {
			return fmt.Sprintf("firmware_version=%q", v)
		}
	case 0x06:
		// ID, length, serial string
		if len(data) >= 3 {
			strLen := int(data[1])
			if 2+strLen <= len(data) {
				return fmt.Sprintf("serial=%q", string(data[2:2+strLen]))
			}
		}
	case 0x08:
		if caps := decodeCapabilities(data); caps != nil {
			return fmt.Sprintf(
				"cols=%d icon=%dx%d panel=%dx%d",
				caps.KeysPerRow, caps.IconWidth, caps.IconHeight,
				caps.PanelWidth, caps.PanelHeight)
		}
	case 0x09:
		if len(data) >= 8 {
			maxPayload := leU16(data, 5)
			return fmt.Sprintf("lcd_screens=%d max_image_payload=%d bytes", data[3], maxPayload)
		}
	case 0x0b:
		if len(data) >= 4 {
			chunkSize := leU16(data, 2)
			return fmt.Sprintf("image_chunk_size=%d bytes (0x%04X)", chunkSize, chunkSize)
		}
	case 0x0c:
		if len(data) >= 2 {
			return fmt.Sprintf("value=0x%02X", data[1])
		}
	}
	return ""
}

// decodeCapabilities attempts to decode feature report 0x08 into a
// DeviceCapabilities struct.
func decodeCapabilities(data []byte) *DeviceCapabilities {
	if len(data) < 11 {
		return nil
	}
	return &DeviceCapabilities{
		KeysPerRow:  int(data[2]),
		IconWidth:   int(leU16(data, 3)),
		IconHeight:  int(leU16(data, 5)),
		PanelWidth:  int(leU16(data, 7)),
		PanelHeight: int(leU16(data, 9)),
		Raw:         strings.ToUpper(hex.EncodeToString(data)),
	}
}

// leU16 reads a little-endian uint16 from data at the given offset.
func leU16(data []byte, offset int) uint16 {
	if offset+1 >= len(data) {
		return 0
	}
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}
