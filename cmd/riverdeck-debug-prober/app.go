package main

import (
	"image/color"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/merith-tk/riverdeck/pkg/prober"
	"github.com/sstallion/go-hid"
)

type step int

const (
	stepSetup step = iota // 1  detect + discover + probe (auto-flows)
	stepInput             // 2  interactive input collection
	stepSave              // 3  save dumps
)

var stepLabels = []string{"1. Setup", "2. Inputs", "3. Save"}

// AppState holds all persistent data across wizard steps.
type AppState struct {
	app    fyne.App
	window fyne.Window

	// Populated during setup
	rawDevices   []hid.DeviceInfo
	probeResults []prober.ProbeResult

	// Per-device input completion state (populated/updated in step 2)
	inputStates []*DeviceInputState

	// Current wizard step content container
	content *fyne.Container
}

func newAppState(a fyne.App, w fyne.Window) *AppState {
	s := &AppState{app: a, window: w}
	s.content = container.NewStack()
	w.SetContent(s.content)
	return s
}

// showStep switches the main window to the given wizard step.
func (s *AppState) showStep(st step) {
	var c fyne.CanvasObject
	switch st {
	case stepSetup:
		c = s.buildSetupStep()
	case stepInput:
		c = s.buildInputStep()
	case stepSave:
		c = s.buildSaveStep()
	default:
		c = widget.NewLabel("Unknown step")
	}

	wrapped := container.NewBorder(
		container.NewVBox(s.buildStepBar(st), widget.NewSeparator()),
		nil, nil, nil,
		c,
	)
	s.content.Objects = []fyne.CanvasObject{wrapped}
	s.content.Refresh()
}

// buildStepBar returns a horizontal breadcrumb showing wizard progress.
func (s *AppState) buildStepBar(current step) fyne.CanvasObject {
	items := make([]fyne.CanvasObject, 0, len(stepLabels)*2-1)
	for i, label := range stepLabels {
		st := step(i)
		if st == current {
			bg := canvas.NewRectangle(color.NRGBA{R: 0x22, G: 0x88, B: 0xcc, A: 0xff})
			bg.SetMinSize(fyne.NewSize(120, 28))
			lbl := widget.NewLabelWithStyle(label, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
			items = append(items, container.NewStack(bg, container.NewCenter(lbl)))
		} else {
			bg := canvas.NewRectangle(theme.DisabledColor())
			bg.SetMinSize(fyne.NewSize(120, 28))
			lbl := widget.NewLabelWithStyle(label, fyne.TextAlignCenter, fyne.TextStyle{})
			items = append(items, container.NewStack(bg, container.NewCenter(lbl)))
		}
		if i < len(stepLabels)-1 {
			items = append(items, widget.NewLabel("    "))
		}
	}
	return container.NewCenter(container.NewHBox(items...))
}
