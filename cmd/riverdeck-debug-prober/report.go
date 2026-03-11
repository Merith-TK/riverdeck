package main

import (
	"fmt"
	"strings"
)

const divider = "================================================================"
const thinLine = "----------------------------------------------------------------"

// PrintReport prints a human-readable diagnostic report for one probed device.
func PrintReport(r ProbeResult) {
	fmt.Println(divider)
	fmt.Printf("  DEVICE PROBE REPORT -- %s\n", r.ProbeTime)
	fmt.Println(thinLine)

	// -- Identity --------------------------------------------------------------
	fmt.Println("  [Identity]")
	fmt.Printf("    Manufacturer : %s\n", fieldOrUnknown(r.Manufacturer))
	fmt.Printf("    Product      : %s\n", fieldOrUnknown(r.Product))
	fmt.Printf("    Serial       : %s\n", fieldOrUnknown(r.Serial))
	fmt.Printf("    Vendor ID    : %s\n", r.VendorIDHex)
	fmt.Printf("    Product ID   : %s\n", r.ProductIDHex)
	fmt.Printf("    HID Path     : %s\n", r.Path)
	fmt.Println()

	// -- Model -----------------------------------------------------------------
	fmt.Println("  [Model]")
	knownStr := "YES"
	if !r.KnownModel {
		knownStr = "NO -- unknown hardware (all values below may be zero/empty)"
	}
	fmt.Printf("    Known Model  : %s\n", knownStr)
	fmt.Printf("    Model Name   : %s\n", r.ModelName)
	if r.Cols > 0 || r.Rows > 0 {
		fmt.Printf("    Layout       : %d cols x %d rows\n", r.Cols, r.Rows)
	} else {
		fmt.Printf("    Layout       : unknown\n")
	}
	fmt.Printf("    Key Count    : %d\n", r.Keys)
	if r.PixelSize > 0 {
		fmt.Printf("    Pixel Size   : %d x %d px\n", r.PixelSize, r.PixelSize)
		fmt.Printf("    Image Format : %s\n", fieldOrUnknown(r.ImageFormat))
	} else {
		fmt.Printf("    Pixel Size   : N/A (no display)\n")
	}
	fmt.Println()

	// -- Firmware --------------------------------------------------------------
	fmt.Println("  [Firmware]")
	fmt.Printf("    Version      : %s\n", fieldOrUnknown(r.Firmware))
	fmt.Println()

	// -- Feature Reports -------------------------------------------------------
	fmt.Println("  [Feature Reports]")

	// Collect unsupported IDs for a compact summary line
	var unsupportedIDs []string
	var errorReports []FeatureReportResult
	var validReports []FeatureReportResult
	for _, fr := range r.FeatureReports {
		if fr.Unsupported {
			unsupportedIDs = append(unsupportedIDs, fr.IDHex)
		} else if fr.Error != "" {
			errorReports = append(errorReports, fr)
		} else {
			validReports = append(validReports, fr)
		}
	}

	for _, fr := range validReports {
		label := ""
		if fr.Label != "" {
			label = "  [" + fr.Label + "]"
		}
		if isAllZero(fr.Raw) {
			fmt.Printf("    %s%s -> (all zeros)\n", fr.IDHex, label)
			continue
		}
		fmt.Printf("    %s%s\n", fr.IDHex, label)
		fmt.Printf("      hex     : %s\n", fr.Raw)
		if fr.ASCII != "" {
			fmt.Printf("      ascii   : %s\n", fr.ASCII)
		}
		if fr.Decoded != "" {
			fmt.Printf("      decoded : %s\n", fr.Decoded)
		}
	}
	for _, fr := range errorReports {
		fmt.Printf("    %s -> UNEXPECTED ERROR: %s\n", fr.IDHex, shortErr(fr.Error))
	}
	if len(unsupportedIDs) > 0 {
		fmt.Printf("    (not present on device: %s)\n", strings.Join(unsupportedIDs, ", "))
	}
	fmt.Println()

	// -- Device Capabilities ---------------------------------------------------
	if r.DeviceCaps != nil {
		dc := r.DeviceCaps
		fmt.Println("  [Device Capabilities (report 0x08)]")
		fmt.Printf("    Keys per row : %d\n", dc.KeysPerRow)
		fmt.Printf("    Icon size    : %d x %d px\n", dc.IconWidth, dc.IconHeight)
		fmt.Printf("    Panel size   : %d x %d px\n", dc.PanelWidth, dc.PanelHeight)
		fmt.Println()
	}

	// -- Key Packet Structure --------------------------------------------------
	fmt.Println("  [Key Packet Structure]")
	if r.KeyPacket != nil {
		kp := r.KeyPacket
		fmt.Printf("    Format       : %s\n", kp.Format)
		fmt.Printf("    Packet Size  : %d bytes\n", kp.PacketSizeBytes)
		fmt.Printf("    Header (hex) : %s\n", kp.HeaderHex)
		fmt.Printf("    Key Offset   : byte %d\n", kp.KeyOffset)
		fmt.Printf("    Key Count    : %d\n", kp.DetectedKeys)
		fmt.Printf("    Sample (hex) : %s\n", truncHex(kp.SampleHex, 128))
	} else {
		fmt.Println("    (no input packets received at idle)")
	}
	fmt.Println()

	// -- Captured Key Events ---------------------------------------------------
	fmt.Println("  [Key Events (during listen window)]")
	if len(r.KeyEvents) == 0 {
		fmt.Println("    (none -- press keys during the listen window to capture them)")
	} else {
		for _, ev := range r.KeyEvents {
			action := "PRESS"
			if !ev.Pressed {
				action = "RELEASE"
			}
			fmt.Printf("    +%4dms  key[%02d] %s\n", ev.RelativeMS, ev.KeyIndex, action)
			fmt.Printf("            pkt: %s\n", truncHex(ev.PacketHex, 128))
		}
	}
	fmt.Println()

	// -- Errors ----------------------------------------------------------------
	if len(r.Errors) > 0 {
		fmt.Println("  [Errors]")
		for _, e := range r.Errors {
			fmt.Printf("    !! %s\n", e)
		}
		fmt.Println()
	}

	// -- Simulator stub --------------------------------------------------------
	fmt.Println("  [Simulator / Support Stub]")
	fmt.Printf("    Add this entry to pkg/streamdeck/models.go if not already present:\n\n")
	imgFmt := r.ImageFormat
	if imgFmt == "" {
		imgFmt = "JPEG /* VERIFY */"
	}
	pixelSize := r.PixelSize
	keys := r.Keys
	cols := r.Cols
	rows := r.Rows
	if r.KeyPacket != nil && !r.KnownModel {
		if keys == 0 {
			keys = r.KeyPacket.DetectedKeys
		}
	}
	fmt.Printf("    %s: {Name: %q, ProductID: %s,\n",
		r.ProductIDHex, r.ModelName, r.ProductIDHex)
	fmt.Printf("        Cols: %d, Rows: %d, Keys: %d,\n", cols, rows, keys)
	fmt.Printf("        PixelSize: %d, ImageFormat: %q},\n", pixelSize, imgFmt)
	fmt.Println()

	fmt.Println(divider)
	fmt.Println()
}

// -- helpers -------------------------------------------------------------------

func fieldOrUnknown(s string) string {
	if strings.TrimSpace(s) == "" {
		return "(unknown)"
	}
	return s
}

func shortErr(s string) string {
	if len(s) > 80 {
		return s[:80] + "..."
	}
	return s
}

func isAllZero(hexStr string) bool {
	for _, c := range hexStr {
		if c != '0' {
			return false
		}
	}
	return true
}

func truncHex(h string, maxChars int) string {
	if len(h) <= maxChars {
		return h
	}
	return h[:maxChars] + "... (" + fmt.Sprintf("%d", len(h)/2) + " bytes total)"
}
