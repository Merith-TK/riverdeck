package main

import (
	"sync"

	"github.com/merith-tk/riverdeck/pkg/prober"
)

// InputKind describes what kind of physical input is being tracked.
type InputKind int

const (
	InputButton     InputKind = iota
	InputDialCW               // clockwise rotation
	InputDialCCW              // counter-clockwise rotation
	InputDialPress            // dial clicked down
	InputTouchPoint           // capacitive touch point (e.g. Neo touch bar)
)

// InputSpec describes a single expected input on a device.
type InputSpec struct {
	Kind  InputKind
	Index int    // key index (for buttons/dial presses) or dial index
	Label string // human-readable e.g. "Button 3", "Dial 1 CW"
	Done  bool   // has this input been triggered at least once?
}

// DeviceInputState tracks interaction progress for one device.
type DeviceInputState struct {
	ProbeResult prober.ProbeResult
	Inputs      []*InputSpec

	// HID streaming
	stopCh  chan struct{}
	eventCh chan prober.CapturedKeyEvent

	// Raw HID packet channel for non-button inputs (dials etc.)
	rawCh chan []byte

	// Collected non-key HID packets captured during the interaction step.
	// Protected by rawMu; merged into ProbeResult before saving.
	rawMu      sync.Mutex
	RawPackets []prober.CapturedRawPacket
}

// modelDialCounts lists how many dials a model has (by ProductID).
// Only models with dials are listed; zero means no dials.
var modelDialCounts = map[uint16]int{
	0x009a: 4, // Stream Deck +
}

// modelTouchCounts lists how many touch points a model has (by ProductID).
// Only models with touch points are listed; zero means none.
var modelTouchCounts = map[uint16]int{
	0x0090: 2, // Stream Deck Neo
}

// buildInputSpec constructs the list of expected inputs for a probed device.
func buildInputSpec(result prober.ProbeResult) []*InputSpec {
	var specs []*InputSpec

	// Buttons
	numKeys := result.Keys
	if numKeys == 0 && result.KeyPacket != nil {
		numKeys = result.KeyPacket.DetectedKeys
	}
	for i := 0; i < numKeys; i++ {
		specs = append(specs, &InputSpec{
			Kind:  InputButton,
			Index: i,
			Label: labelButton(i),
		})
	}

	// Dials (if this model has them)
	dialCount := modelDialCounts[result.ProductID]
	for d := 0; d < dialCount; d++ {
		specs = append(specs, &InputSpec{Kind: InputDialCW, Index: d,
			Label: labelDial(d, "CW")})
		specs = append(specs, &InputSpec{Kind: InputDialCCW, Index: d,
			Label: labelDial(d, "CCW")})
		specs = append(specs, &InputSpec{Kind: InputDialPress, Index: d,
			Label: labelDial(d, "Press")})
	}

	// Touch points (if this model has them)
	touchCount := modelTouchCounts[result.ProductID]
	for t := 0; t < touchCount; t++ {
		specs = append(specs, &InputSpec{Kind: InputTouchPoint, Index: t,
			Label: labelTouch(t)})
	}

	return specs
}

func labelButton(i int) string {
	return "Button " + itoa(i+1)
}

func labelDial(d int, dir string) string {
	return "Dial " + itoa(d+1) + " " + dir
}

func labelTouch(t int) string {
	return "Touch " + itoa(t+1)
}

// allDone returns true when every InputSpec in the slice is Done.
func allDone(specs []*InputSpec) bool {
	for _, sp := range specs {
		if !sp.Done {
			return false
		}
	}
	return len(specs) > 0
}

func itoa(n int) string {
	return intToString(n)
}

func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var buf [20]byte
	pos := len(buf)
	for n > 0 {
		pos--
		buf[pos] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		pos--
		buf[pos] = '-'
	}
	return string(buf[pos:])
}
