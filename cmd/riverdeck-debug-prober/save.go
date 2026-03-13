package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/dialog"
	"fyne.io/fyne/v2/widget"
	"github.com/merith-tk/riverdeck/pkg/prober"
)

func (s *AppState) buildSaveStep() fyne.CanvasObject {
	header := widget.NewLabelWithStyle(
		"Save probe dumps",
		fyne.TextAlignLeading,
		fyne.TextStyle{Bold: true},
	)

	// Deduplicate probe results by model name, merging in raw packets
	// collected during the interaction step.
	seen := map[string]bool{}
	inputByModel := map[string]*DeviceInputState{}
	for _, dis := range s.inputStates {
		inputByModel[dis.ProbeResult.ModelName] = dis
	}
	var toSave []prober.ProbeResult
	for _, r := range s.probeResults {
		if seen[r.ModelName] {
			continue
		}
		seen[r.ModelName] = true
		// Merge raw packets from the GUI interaction step into the probe result.
		if dis, ok := inputByModel[r.ModelName]; ok {
			dis.rawMu.Lock()
			r.RawPackets = append(r.RawPackets, dis.RawPackets...)
			dis.rawMu.Unlock()
		}
		toSave = append(toSave, r)
	}

	var outDir string
	if wd, err := os.Getwd(); err == nil {
		outDir = wd
	}

	dirLabel := widget.NewLabel("Output directory: " + outDir)
	dirLabel.Wrapping = fyne.TextWrapWord

	logBox := widget.NewLabel("")
	logBox.Wrapping = fyne.TextWrapWord

	statusLabel := widget.NewLabelWithStyle("", fyne.TextAlignLeading, fyne.TextStyle{})

	// Summary of what will be saved.
	var summaryLines []string
	for _, r := range toSave {
		summaryLines = append(summaryLines, fmt.Sprintf(
			"  * %s -- %s  (keys: %d, firmware: %s)",
			modelFileName(r.ModelName), r.ModelName, r.Keys, r.Firmware))
	}
	summaryLabel := widget.NewLabel(strings.Join(summaryLines, "\n"))

	browseBtn := widget.NewButton("Browse...", func() {
		d := dialog.NewFolderOpen(func(uri fyne.ListableURI, err error) {
			if err != nil || uri == nil {
				return
			}
			outDir = uri.Path()
			dirLabel.SetText("Output directory: " + outDir)
		}, s.window)
		d.Show()
	})

	saveBtn := widget.NewButton("Save all dumps", nil)
	saveBtn.OnTapped = func() {
		saveBtn.Disable()
		logBox.SetText("")

		var log strings.Builder
		ok := 0
		for _, r := range toSave {
			fname := modelFileName(r.ModelName) + "_" + sanitizeForFilename(time.Now().Format("20060102")) + ".json"
			fullPath := filepath.Join(outDir, fname)

			if err := writeProbeJSON(fullPath, r); err != nil {
				log.WriteString(fmt.Sprintf("ERROR: %s -> %v\n", fname, err))
			} else {
				log.WriteString(fmt.Sprintf("Saved: %s\n", fullPath))
				ok++
			}
		}
		logBox.SetText(log.String())

		if ok == len(toSave) {
			statusLabel.SetText(fmt.Sprintf("Done! %d file(s) saved.", ok))
		} else {
			statusLabel.SetText(fmt.Sprintf("Saved %d / %d files. Some errors occurred.", ok, len(toSave)))
		}
		saveBtn.Enable()
	}

	backBtn := widget.NewButton("<- Back", func() { s.showStep(stepInput) })
	finishBtn := widget.NewButton("Finish", func() { s.app.Quit() })

	nav := container.NewBorder(nil, nil, backBtn, finishBtn)

	body := container.NewVBox(
		header,
		widget.NewSeparator(),
		widget.NewLabel("The following model dumps will be saved:"),
		summaryLabel,
		widget.NewSeparator(),
		dirLabel,
		browseBtn,
		saveBtn,
		widget.NewSeparator(),
		statusLabel,
		logBox,
	)

	return container.NewBorder(nil, nav, nil, nil, container.NewVScroll(body))
}

// writeProbeJSON serialises a single ProbeResult to a JSON file, wrapped in a slice
// to match the existing CLI prober format.
func writeProbeJSON(path string, r prober.ProbeResult) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()

	enc := json.NewEncoder(f)
	enc.SetIndent("", "  ")
	return enc.Encode([]prober.ProbeResult{r})
}

// modelFileName converts a model name like "Stream Deck MK.2" to "stream-deck-mk-2".
func modelFileName(name string) string {
	return sanitizeForFilename(name)
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

func sanitizeForFilename(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	for strings.Contains(s, "--") {
		s = strings.ReplaceAll(s, "--", "-")
	}
	return s
}
