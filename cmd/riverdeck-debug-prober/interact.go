package main

import (
	"encoding/hex"
	"fmt"
	"image/color"
	"strings"
	"sync"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
	"fyne.io/fyne/v2/widget"
	"github.com/merith-tk/riverdeck/pkg/prober"
	"github.com/sstallion/go-hid"
)

func (s *AppState) buildInputStep() fyne.CanvasObject {
	// Build input states for each device (deduplicated by model name)
	seen := map[string]bool{}
	s.inputStates = nil
	for _, r := range s.probeResults {
		if seen[r.ModelName] {
			continue
		}
		seen[r.ModelName] = true
		dis := &DeviceInputState{
			ProbeResult: r,
			Inputs:      buildInputSpec(r),
			stopCh:      make(chan struct{}),
			eventCh:     make(chan prober.CapturedKeyEvent, 64),
			rawCh:       make(chan []byte, 64),
		}
		s.inputStates = append(s.inputStates, dis)
	}

	if len(s.inputStates) == 0 {
		return container.NewVBox(
			widget.NewLabel("No devices to test."),
			widget.NewButton("<- Back to Setup", func() { s.showStep(stepSetup) }),
		)
	}

	tabs := container.NewAppTabs()
	var tabItems []*container.TabItem

	nextBtn := widget.NewButton("Continue to Save ->", func() { s.showStep(stepSave) })

	checkAllDone := func() {
		for _, dis := range s.inputStates {
			if !allDone(dis.Inputs) {
				nextBtn.Disable()
				return
			}
		}
		nextBtn.Enable()
		go func() {
			time.Sleep(1500 * time.Millisecond)
			fyne.Do(func() { s.showStep(stepSave) })
		}()
	}

	for _, dis := range s.inputStates {
		tabItem := s.buildDeviceInputTab(dis, checkAllDone)
		tabItems = append(tabItems, tabItem)
	}
	tabs.SetItems(tabItems)

	checkAllDone()

	backBtn := widget.NewButton("<- Back to Setup", func() {
		for _, dis := range s.inputStates {
			select {
			case <-dis.stopCh:
			default:
				close(dis.stopCh)
			}
		}
		s.showStep(stepSetup)
	})

	header := widget.NewLabelWithStyle(
		"Press every button and interact with every input on each device tab.",
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)
	hint := widget.NewLabel("Buttons: press once.  Dials: rotate both CW and CCW, then press down.")
	hint.Wrapping = fyne.TextWrapWord

	nav := container.NewBorder(nil, nil, backBtn, nextBtn)

	return container.NewBorder(
		container.NewVBox(header, hint, widget.NewSeparator()),
		nav,
		nil, nil,
		tabs,
	)
}

func (s *AppState) buildDeviceInputTab(dis *DeviceInputState, onChange func()) *container.TabItem {
	r := dis.ProbeResult

	title := r.ModelName
	if title == "" {
		title = r.Product
	}

	progressLabel := widget.NewLabel("")
	updateProgress := func() {
		done := 0
		total := len(dis.Inputs)
		for _, sp := range dis.Inputs {
			if sp.Done {
				done++
			}
		}
		if done == total && total > 0 {
			progressLabel.SetText("All inputs received!")
		} else {
			progressLabel.SetText(fmt.Sprintf("Progress: %d / %d inputs", done, total))
		}
	}
	updateProgress()

	inputCards := make([]*inputCard, len(dis.Inputs))
	var cardObjects []fyne.CanvasObject
	for i, sp := range dis.Inputs {
		ic := newInputCard(sp)
		inputCards[i] = ic
		cardObjects = append(cardObjects, ic.renderObj())
	}

	cols := 5
	if len(dis.Inputs) <= 6 {
		cols = 3
	} else if len(dis.Inputs) > 20 {
		cols = 6
	}
	grid := container.NewGridWithColumns(cols, cardObjects...)

	statusLabel := widget.NewLabel("Connecting to device...")
	statusLabel.Wrapping = fyne.TextWrapWord

	skipBtn := widget.NewButton("Skip this device", func() {
		for i, sp := range dis.Inputs {
			sp.Done = true
			inputCards[i].setDone(true)
		}
		updateProgress()
		onChange()
	})

	var mu sync.Mutex

	go func() {
		dev, err := hid.OpenPath(r.Path)
		if err != nil {
			errMsg := fmt.Sprintf("Cannot open device: %v\nClose conflicting software and restart this step.", err)
			fyne.Do(func() { statusLabel.SetText(errMsg) })
			return
		}
		defer dev.Close()
		fyne.Do(func() { statusLabel.SetText("Listening for inputs...") })

		dialLastValue := map[int]int{}
		buf := make([]byte, 512)
		keyOffset := -1
		start := time.Now()

		for {
			select {
			case <-dis.stopCh:
				return
			default:
			}

			n, err := dev.ReadWithTimeout(buf, 100)
			if err != nil || n == 0 {
				continue
			}

			pkt := make([]byte, n)
			copy(pkt, buf[:n])

			if keyOffset == -1 {
				_, ko, _ := prober.DetectKeyFormat(pkt)
				if ko == 0 {
					keyOffset = 4
				} else {
					keyOffset = ko
				}
			}

			_, _, keyCount := prober.DetectKeyFormat(pkt)
			isButtonPkt := keyCount > 0 && n >= keyOffset

			if isButtonPkt {
				var changed []*inputCard
				mu.Lock()
				for i := 0; i < r.Keys && keyOffset+i < n; i++ {
					if pkt[keyOffset+i] != 0 {
						for j, sp := range dis.Inputs {
							if sp.Kind == InputButton && sp.Index == i && !sp.Done {
								sp.Done = true
								changed = append(changed, inputCards[j])
								break
							}
						}
					}
				}
				mu.Unlock()
				if len(changed) > 0 {
					fyne.Do(func() {
						for _, ic := range changed {
							ic.setDone(true)
						}
						updateProgress()
						onChange()
					})
				}
			} else if len(pkt) >= 5 && pkt[0] == 0x01 && pkt[1] == 0x03 {
				// Stream Deck + dial packet: 01 03 <dial_idx> <event_type> <value>
				dialIdx := int(pkt[2])
				evType := pkt[3]
				value := int8(pkt[4])

				var changedCard *inputCard
				mu.Lock()
				if evType == 0x01 {
					for i, sp := range dis.Inputs {
						if sp.Kind == InputDialPress && sp.Index == dialIdx && !sp.Done {
							sp.Done = true
							changedCard = inputCards[i]
							break
						}
					}
				} else if evType == 0x00 {
					_ = dialLastValue[dialIdx]
					dialLastValue[dialIdx] = int(value)
					var kind InputKind
					if value > 0 {
						kind = InputDialCW
					} else if value < 0 {
						kind = InputDialCCW
					}
					if value != 0 {
						for i, sp := range dis.Inputs {
							if sp.Kind == kind && sp.Index == dialIdx && !sp.Done {
								sp.Done = true
								changedCard = inputCards[i]
								break
							}
						}
					}
				}
				mu.Unlock()
				if changedCard != nil {
					fyne.Do(func() {
						changedCard.setDone(true)
						updateProgress()
						onChange()
					})
				}
			} else {
				// Unknown packet -- save full hex to RawPackets for analysis and
				// show a truncated preview in the status label.
				pktHex := strings.ToUpper(hex.EncodeToString(pkt))
				dis.rawMu.Lock()
				dis.RawPackets = append(dis.RawPackets, prober.CapturedRawPacket{
					RelativeMS: time.Since(start).Milliseconds(),
					Length:     n,
					PacketHex:  pktHex,
				})
				dis.rawMu.Unlock()

				display := pktHex
				if len(display) > 48 {
					display = display[:48] + "..."
				}
				fyne.Do(func() { statusLabel.SetText("Last raw pkt: " + display) })
			}
		}
	}()

	body := container.NewVBox(
		widget.NewLabel(fmt.Sprintf("Device: %s  |  Keys: %d  |  Firmware: %s",
			r.ModelName, r.Keys, r.Firmware)),
		widget.NewSeparator(),
		progressLabel,
		statusLabel,
		widget.NewSeparator(),
		grid,
		widget.NewSeparator(),
		skipBtn,
	)

	return container.NewTabItem(title, container.NewVScroll(body))
}

// inputCard is a coloured tile indicating done/pending state for one input.
type inputCard struct {
	spec  *InputSpec
	bg    *canvas.Rectangle
	label *widget.Label
	cont  fyne.CanvasObject
}

func newInputCard(sp *InputSpec) *inputCard {
	bg := canvas.NewRectangle(theme.DisabledColor())
	bg.SetMinSize(fyne.NewSize(80, 48))
	lbl := widget.NewLabelWithStyle(sp.Label, fyne.TextAlignCenter, fyne.TextStyle{Bold: true})
	c := container.NewStack(bg, container.NewCenter(lbl))
	return &inputCard{spec: sp, bg: bg, label: lbl, cont: c}
}

func (ic *inputCard) renderObj() fyne.CanvasObject {
	return ic.cont
}

func (ic *inputCard) setDone(done bool) {
	if done {
		ic.bg.FillColor = color.NRGBA{R: 0x22, G: 0xaa, B: 0x44, A: 0xff}
	} else {
		ic.bg.FillColor = theme.DisabledColor()
	}
	ic.bg.Refresh()
}
