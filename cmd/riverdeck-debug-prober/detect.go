package main

import (
	"fmt"
	"os/exec"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
	"github.com/merith-tk/riverdeck/pkg/prober"
	"github.com/merith-tk/riverdeck/pkg/streamdeck"
)

// conflictProcesses lists known Elgato / OpenDeck process names (case-insensitive).
var conflictProcesses = []string{
	"Stream Deck.exe",
	"StreamDeck.exe",
	"ElgatoStreamDeck.exe",
	"OpenDeck.exe",
	"opendeck.exe",
}

func detectConflictingProcesses() []string {
	out, err := exec.Command("tasklist", "/FO", "CSV", "/NH").Output()
	if err != nil {
		return nil
	}
	lower := strings.ToLower(string(out))
	var found []string
	for _, name := range conflictProcesses {
		if strings.Contains(lower, strings.ToLower(name)) {
			found = append(found, name)
		}
	}
	return found
}

// buildSetupStep is the combined detect+discover+probe step.
// It auto-flows through all phases and auto-advances to stepInput on success.
func (s *AppState) buildSetupStep() fyne.CanvasObject {
	header := widget.NewLabelWithStyle(
		"Starting up...",
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	conflictBanner := widget.NewLabel("")
	conflictBanner.Wrapping = fyne.TextWrapWord
	conflictBanner.Hide()

	progress := widget.NewProgressBar()
	progress.Min = 0
	progress.Max = 3

	logBox := widget.NewLabel("")
	logBox.Wrapping = fyne.TextWrapWord

	nextBtn := widget.NewButton("Continue to Inputs ->", func() { s.showStep(stepInput) })
	nextBtn.Disable()

	rescanBtn := widget.NewButton("Rescan", func() { s.showStep(stepSetup) })
	rescanBtn.Hide()

	appendLog := func(msg string) {
		existing := logBox.Text
		if existing != "" {
			existing += "\n"
		}
		logBox.SetText(existing + msg)
	}

	go func() {
		// Phase 1: conflict detection
		fyne.Do(func() { header.SetText("Checking for conflicting software...") })
		conflicts := detectConflictingProcesses()
		fyne.Do(func() {
			progress.SetValue(1)
			if len(conflicts) > 0 {
				conflictBanner.SetText(
					"Warning: conflicting software detected: " + strings.Join(conflicts, ", ") +
						"\nClose it for best results. You may still continue.")
				conflictBanner.Show()
				appendLog("WARN conflicting software: " + strings.Join(conflicts, ", "))
			} else {
				appendLog("OK   no conflicting software detected")
			}
		})

		// Phase 2: discover devices
		fyne.Do(func() { header.SetText("Scanning for Elgato devices...") })
		if err := streamdeck.Init(); err != nil {
			fyne.Do(func() {
				header.SetText("HID init failed: " + err.Error())
				appendLog("ERROR HID init: " + err.Error())
				rescanBtn.Show()
			})
			return
		}
		devices, total, err := prober.EnumerateElgato()
		if err != nil {
			fyne.Do(func() {
				header.SetText("Enumeration failed: " + err.Error())
				appendLog("ERROR enumeration: " + err.Error())
				rescanBtn.Show()
			})
			return
		}
		s.rawDevices = devices
		if len(devices) == 0 {
			fyne.Do(func() {
				progress.SetValue(2)
				header.SetText(fmt.Sprintf("No Elgato devices found (scanned %d HID devices).", total))
				appendLog(fmt.Sprintf("INFO  scanned %d HID devices  no Elgato devices found", total))
				appendLog("      Make sure devices are plugged in and try again.")
				rescanBtn.Show()
			})
			return
		}
		fyne.Do(func() {
			progress.SetValue(2)
			progress.Max = float64(2 + len(devices))
			appendLog(fmt.Sprintf("OK   found %d Elgato device(s) (scanned %d total HID)", len(devices), total))
			header.SetText(fmt.Sprintf("Probing %d device(s)...", len(devices)))
		})

		// Phase 3: probe each device
		s.probeResults = nil
		seen := map[string]bool{}
		errCount := 0

		for i, raw := range devices {
			model, known := streamdeck.LookupModel(raw.ProductID)
			name := raw.ProductStr
			if known {
				name = model.Name
			}
			nameCopy := name
			iCopy := i
			fyne.Do(func() {
				header.SetText(fmt.Sprintf("Probing [%d/%d]: %s...", iCopy+1, len(devices), nameCopy))
			})

			result := prober.ProbeDevice(raw, 0, false)
			s.probeResults = append(s.probeResults, result)

			resultCopy := result
			fyne.Do(func() {
				progress.SetValue(float64(2 + iCopy + 1))
				if seen[resultCopy.ModelName] {
					appendLog(fmt.Sprintf("SKIP %s  duplicate model", nameCopy))
				} else {
					seen[resultCopy.ModelName] = true
					if len(resultCopy.Errors) > 0 {
						appendLog(fmt.Sprintf("WARN %s  %v", nameCopy, resultCopy.Errors))
						errCount++
					} else {
						appendLog(fmt.Sprintf("OK   %s  firmware %s, %d keys",
							nameCopy, resultCopy.Firmware, resultCopy.Keys))
					}
				}
			})
		}

		uniqueCount := len(seen)
		fyne.Do(func() {
			header.SetText(fmt.Sprintf("Setup complete  %d device(s), %d unique model(s).", len(devices), uniqueCount))
			rescanBtn.Show()
			nextBtn.Enable()
		})

		// Auto-advance after a short pause when everything went clean.
		if errCount == 0 {
			time.Sleep(1500 * time.Millisecond)
			fyne.Do(func() { s.showStep(stepInput) })
		}
	}()

	nav := container.NewBorder(nil, nil, rescanBtn, nextBtn)

	body := container.NewVBox(
		header,
		progress,
		conflictBanner,
		widget.NewSeparator(),
		widget.NewLabel("Log:"),
	)

	return container.NewBorder(body, nav, nil, nil, container.NewVScroll(logBox))
}
