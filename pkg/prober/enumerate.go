package prober

import (
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
	"github.com/sstallion/go-hid"
)

// EnumerateElgato returns all connected Elgato HID devices.
func EnumerateElgato() ([]hid.DeviceInfo, int, error) {
	var rawDevices []hid.DeviceInfo
	var allCount int
	err := hid.Enumerate(0x0000, 0x0000, func(info *hid.DeviceInfo) error {
		allCount++
		if info.VendorID == streamdeck.VendorID {
			rawDevices = append(rawDevices, *info)
		}
		return nil
	})
	return rawDevices, allCount, err
}
