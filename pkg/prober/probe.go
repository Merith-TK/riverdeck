package prober

import (
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/sstallion/go-hid"
)

// curatedReportIDs is the curated list of feature report IDs to always probe.
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
		if !known && kp.DetectedKeys > 0 {
			r.Keys = kp.DetectedKeys
		}
	}

	// -- Live key-event capture ------------------------------------------------
	if listenDur > 0 {
		r.KeyEvents = CaptureKeyEvents(dev, r.Keys, listenDur)
	}

	return r
}

// probeKeyPacket reads up to 5 input packets at idle and analyses the byte layout.
func probeKeyPacket(dev *hid.Device) *KeyPacketInfo {
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

	kp.Format, kp.KeyOffset, kp.DetectedKeys = DetectKeyFormat(first)
	if kp.KeyOffset > 0 && kp.KeyOffset <= len(first) {
		kp.HeaderHex = strings.ToUpper(hex.EncodeToString(first[:kp.KeyOffset]))
	}

	return kp
}

// DetectKeyFormat examines one raw HID input packet and returns the format name,
// key-data byte offset, and number of keys.
func DetectKeyFormat(pkt []byte) (format string, keyOffset int, keyCount int) {
	if len(pkt) < 2 || pkt[0] != 0x01 {
		return "unknown", 0, 0
	}

	// V2: 01 00 NN 00 [keys...]
	if len(pkt) >= 5 && pkt[1] == 0x00 && pkt[3] == 0x00 {
		nn := int(pkt[2])
		if nn >= 1 && nn <= 64 {
			return "V2", 4, nn
		}
	}

	// V1: 01 [keys...]
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

// CaptureKeyEvents listens for HID input reports for the given duration and
// records every key state change as a CapturedKeyEvent.
func CaptureKeyEvents(dev *hid.Device, expectedKeys int, dur time.Duration) []CapturedKeyEvent {
	if expectedKeys <= 0 {
		expectedKeys = 32
	}

	var events []CapturedKeyEvent
	prevState := make([]bool, expectedKeys)
	start := time.Now()
	deadline := start.Add(dur)

	buf := make([]byte, 512)
	keyOffset := -1

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

		if keyOffset == -1 {
			_, keyOffset, _ = DetectKeyFormat(buf[:n])
			if keyOffset == 0 {
				keyOffset = 4
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

// OpenDevice opens a HID device by path and returns it for streaming reads.
func OpenDevice(path string) (*hid.Device, error) {
	return hid.OpenPath(path)
}

// StreamEvents reads raw HID packets from an already-opened device and sends
// CapturedKeyEvents to the provided channel. It returns when ctx is cancelled.
// expectedKeys should come from the probe result.
func StreamEvents(dev *hid.Device, expectedKeys int, out chan<- CapturedKeyEvent, stop <-chan struct{}) {
	if expectedKeys <= 0 {
		expectedKeys = 32
	}
	prevState := make([]bool, expectedKeys)
	buf := make([]byte, 512)
	keyOffset := -1
	start := time.Now()

	for {
		select {
		case <-stop:
			return
		default:
		}

		n, err := dev.ReadWithTimeout(buf, 100*time.Millisecond)
		if err != nil || n == 0 {
			continue
		}
		pktHex := strings.ToUpper(hex.EncodeToString(buf[:n]))

		if keyOffset == -1 {
			_, keyOffset, _ = DetectKeyFormat(buf[:n])
			if keyOffset == 0 {
				keyOffset = 4
			}
		}

		for i := 0; i < expectedKeys && keyOffset+i < n; i++ {
			pressed := buf[keyOffset+i] != 0
			if pressed != prevState[i] {
				select {
				case out <- CapturedKeyEvent{
					RelativeMS: time.Since(start).Milliseconds(),
					KeyIndex:   i,
					Pressed:    pressed,
					PacketHex:  pktHex,
				}:
				default:
				}
				prevState[i] = pressed
			}
		}
	}
}

// -- helpers -------------------------------------------------------------------

func extractFirmware(data []byte) string {
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

func isUnsupportedReportErr(err error) bool {
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "0x00000057") ||
		strings.Contains(s, "parameter is incorrect") ||
		strings.Contains(s, "invalid parameter")
}

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
	if len(s) < 3 {
		return ""
	}
	return s
}

func decodeReport(id byte, data []byte) string {
	switch id {
	case 0x03:
		if len(data) >= 2 {
			return fmt.Sprintf("state=0x%02X", data[1])
		}
	case 0x04, 0x07:
		for start := 2; start < len(data)-4; start++ {
			if data[start] >= 0x30 && data[start] <= 0x39 ||
				(data[start] >= 'A' && data[start] <= 'Z') ||
				(data[start] >= 'a' && data[start] <= 'z') {
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

func leU16(data []byte, offset int) uint16 {
	if offset+1 >= len(data) {
		return 0
	}
	return uint16(data[offset]) | uint16(data[offset+1])<<8
}
