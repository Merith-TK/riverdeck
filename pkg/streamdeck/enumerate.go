package streamdeck

import (
	"fmt"

	"github.com/sstallion/go-hid"
)

// DeviceInfo contains information about a connected Stream Deck.
type DeviceInfo struct {
	Path         string
	Serial       string
	Manufacturer string
	Product      string
	Model        Model
	Firmware     string
}

// Init initializes the HID library. Must be called before using other functions.
func Init() error {
	return hid.Init()
}

// Exit cleans up the HID library. Should be called when done.
func Exit() error {
	return hid.Exit()
}

// Enumerate finds all connected Stream Deck devices.
func Enumerate() ([]DeviceInfo, error) {
	var devices []DeviceInfo

	err := hid.Enumerate(VendorID, 0x0000, func(info *hid.DeviceInfo) error {
		model, known := LookupModel(info.ProductID)
		if !known {
			// Unknown Stream Deck variant
			model = Model{
				Name:      fmt.Sprintf("Unknown Stream Deck (PID: 0x%04X)", info.ProductID),
				ProductID: info.ProductID,
			}
		}

		devInfo := DeviceInfo{
			Path:         info.Path,
			Serial:       info.SerialNbr,
			Manufacturer: info.MfrStr,
			Product:      info.ProductStr,
			Model:        model,
		}

		// Try to get firmware version
		dev, err := hid.OpenPath(info.Path)
		if err == nil {
			devInfo.Firmware = getFirmwareVersion(dev)
			dev.Close()
		}

		devices = append(devices, devInfo)
		return nil
	})

	return devices, err
}

// getFirmwareVersion reads the firmware version from the device.
func getFirmwareVersion(dev *hid.Device) string {
	data := make([]byte, 32)
	data[0] = 0x05 // Firmware version command

	_, err := dev.GetFeatureReport(data)
	if err != nil {
		return "unknown"
	}

	// Firmware string starts at offset 6 for MK.2/V2 devices
	for i := 6; i < len(data); i++ {
		if data[i] == 0 {
			return string(data[6:i])
		}
	}
	return string(data[6:])
}

// PrintDeviceInfo prints detailed information about a device to stdout.
func PrintDeviceInfo(info DeviceInfo) {
	fmt.Println("===================================================")
	fmt.Printf("  Model:        %s\n", info.Model.Name)
	fmt.Printf("  Product:      %s\n", info.Product)
	fmt.Printf("  Manufacturer: %s\n", info.Manufacturer)
	fmt.Printf("  Serial:       %s\n", info.Serial)
	fmt.Printf("  Firmware:     %s\n", info.Firmware)
	fmt.Printf("  Product ID:   0x%04X\n", info.Model.ProductID)
	fmt.Println("---------------------------------------------------")
	fmt.Printf("  Layout:       %d columns x %d rows\n", info.Model.Cols, info.Model.Rows)
	fmt.Printf("  Total Keys:   %d\n", info.Model.Keys)
	if info.Model.PixelSize > 0 {
		fmt.Printf("  Icon Size:    %d x %d pixels\n", info.Model.PixelSize, info.Model.PixelSize)
		fmt.Printf("  Image Format: %s\n", info.Model.ImageFormat)
	} else {
		fmt.Println("  Icon Size:    N/A (no display)")
	}
	fmt.Println("===================================================")
}
