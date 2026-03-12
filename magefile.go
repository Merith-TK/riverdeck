//go:build mage

package main

import (
	"fmt"
	"image"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/srwiley/oksvg"
	"github.com/srwiley/rasterx"
)

// gremlins maps Unicode gremlin characters to their ASCII replacements.
var gremlins = []struct {
	unicode string
	ascii   string
	name    string
}{
	// Quotes
	{"\u2018", "'", "left single quotation mark"},
	{"\u2019", "'", "right single quotation mark"},
	{"\u201A", "'", "single low-9 quotation mark"},
	{"\u201B", "'", "single high-reversed-9 quotation mark"},
	{"\u201C", "\"", "left double quotation mark"},
	{"\u201D", "\"", "right double quotation mark"},
	{"\u201E", "\"", "double low-9 quotation mark"},
	{"\u201F", "\"", "double high-reversed-9 quotation mark"},
	{"\u00B4", "'", "acute accent"},
	{"\u02BC", "'", "modifier letter apostrophe"},
	{"\u02B9", "'", "modifier letter prime"},
	{"\u2032", "'", "prime"},
	{"\u2033", "\"", "double prime"},
	// Dashes and hyphens
	{"\u2010", "-", "hyphen"},
	{"\u2011", "-", "non-breaking hyphen"},
	{"\u2012", "-", "figure dash"},
	{"\u2013", "-", "en dash"},
	{"\u2014", "--", "em dash"},
	{"\u2015", "--", "horizontal bar"},
	{"\u2212", "-", "minus sign"},
	{"\u00AD", "-", "soft hyphen"},
	// Ellipsis
	{"\u2026", "...", "horizontal ellipsis"},
	// Spaces
	{"\u00A0", " ", "non-breaking space"},
	{"\u202F", " ", "narrow no-break space"},
	{"\u2009", " ", "thin space"},
	{"\u200A", " ", "hair space"},
	{"\u3000", " ", "ideographic space"},
	// Zero-width characters (remove entirely)
	{"\uFEFF", "", "BOM / zero width no-break space"},
	{"\u200B", "", "zero width space"},
	{"\u200C", "", "zero width non-joiner"},
	{"\u200D", "", "zero width joiner"},
	{"\u2060", "", "word joiner"},
	// Bullets and list markers
	{"\u2022", "*", "bullet"},
	{"\u2023", ">", "triangular bullet"},
	{"\u25E6", "*", "white bullet"},
	// Misc
	{"\u2248", "~=", "almost equal to"},
	{"\u2260", "!=", "not equal to"},
	{"\u2264", "<=", "less-than or equal to"},
	{"\u2265", ">=", "greater-than or equal to"},
	{"\u2550", "=", "box drawings double horizontal"},
	{"\u00D7", "x", "multiplication sign"},
	{"\u2192", "->", "rightwards arrow"},
	{"\u2713", "v", "check mark"},
}

// sourceExtensions is the set of file extensions to process.
var sourceExtensions = map[string]bool{
	".go":   true,
	".md":   true,
	".txt":  true,
	".yaml": true,
	".yml":  true,
	".json": true,
	".lua":  true,
	".toml": true,
}

// --------------------------------------------------------------------------
// Build tasks
// --------------------------------------------------------------------------

// iconSizes lists the PNG sizes (px) generated from the SVG for use by go-winres.
var iconSizes = []int{16, 32, 48, 64, 128, 256}

// Icons rasterises resources/icon.svg into PNG files under resources/icons/
// at all standard sizes. These PNGs are consumed by WinRes to build the
// Windows .exe icon.
func Icons() error {
	const svgPath = "resources/icon.svg"
	const outDir = "resources/icons"

	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	f, err := os.Open(svgPath)
	if err != nil {
		return fmt.Errorf("open svg: %w", err)
	}
	defer f.Close()

	icon, err := oksvg.ReadIconStream(f)
	if err != nil {
		return fmt.Errorf("parse svg: %w", err)
	}

	for _, sz := range iconSizes {
		icon.SetTarget(0, 0, float64(sz), float64(sz))
		rgba := image.NewRGBA(image.Rect(0, 0, sz, sz))
		scanner := rasterx.NewScannerGV(sz, sz, rgba, rgba.Bounds())
		dasher := rasterx.NewDasher(sz, sz, scanner)
		icon.Draw(dasher, 1.0)

		outPath := filepath.Join(outDir, fmt.Sprintf("icon_%d.png", sz))
		out, err := os.Create(outPath)
		if err != nil {
			return fmt.Errorf("create %s: %w", outPath, err)
		}
		if encErr := png.Encode(out, rgba); encErr != nil {
			out.Close()
			return fmt.Errorf("encode %s: %w", outPath, encErr)
		}
		out.Close()
		fmt.Printf("  wrote %s\n", outPath)
	}
	return nil
}

// WinRes generates the Windows resource file (rsrc_windows_amd64.syso) inside
// cmd/riverdeck/ using go-winres.  The .syso is picked up automatically by
// `go build` on Windows targets to embed the icon and manifest into the exe.
//
// Requires: go install github.com/tc-hib/go-winres@latest
func WinRes() error {
	if err := Icons(); err != nil {
		return err
	}
	fmt.Println(">>> generating Windows resource file (.syso)...")
	return runInDir("cmd/riverdeck", "go-winres", "make", "--arch", "amd64,386")
}

// WailsBuild compiles the Wails editor (cmd/riverdeck-wails) using the Wails
// CLI.  The Wails CLI requires wails.json in the working directory, so the
// command is executed inside cmd/riverdeck-wails.
//
// Requires: go install github.com/wailsapp/wails/v2/cmd/wails@latest
func WailsBuild() error {
	fmt.Println(">>> building riverdeck-wails via Wails CLI...")
	return runInDir("cmd/riverdeck-wails", "wails", "build")
}

// WinResAll generates Windows resource files (.syso) for all binaries that
// have a winres/ directory.  This embeds the icon and manifest into the exe.
//
// Requires: go install github.com/tc-hib/go-winres@latest
func WinResAll() error {
	if err := Icons(); err != nil {
		return err
	}
	dirs := []string{
		"cmd/riverdeck",
		"cmd/riverdeck-wails",
	}
	for _, dir := range dirs {
		winresDir := filepath.Join(dir, "winres")
		if _, err := os.Stat(winresDir); os.IsNotExist(err) {
			continue
		}
		fmt.Printf(">>> generating .syso for %s...\n", dir)
		if err := runInDir(dir, "go-winres", "make", "--arch", "amd64,386"); err != nil {
			return fmt.Errorf("winres %s: %w", dir, err)
		}
	}
	return nil
}

// Build compiles all binaries for the current OS/arch into dist/<goos>_<goarch>/.
// On Windows it also regenerates exe icons/manifests (.syso) first.
//
// Standard binaries are built with -trimpath.  Wails GUI binaries are
// additionally built with -tags desktop,production and -ldflags "-w -s -H
// windowsgui" so the console window is hidden on Windows.
//
// Output layout (example):
//
//	dist/windows_amd64/  - riverdeck.exe, riverdeck-debug-prober.exe, riverdeck-wails.exe, ...
//	dist/linux_amd64/    - riverdeck,     riverdeck-debug-prober,     riverdeck-wails, ...
func Build() error {
	goos := runtime.GOOS
	goarch := runtime.GOARCH

	if goos == "windows" {
		if err := WinResAll(); err != nil {
			return fmt.Errorf("winres: %w", err)
		}
	}

	// Standard (CLI / headless) binaries.
	standardBinaries := []string{
		"./cmd/riverdeck",
		"./cmd/riverdeck-debug-prober",
		"./cmd/riverdeck-simulator",
	}

	// Wails GUI binaries - need production tags and windowsgui ldflags.
	wailsBinaries := []string{
		"./cmd/riverdeck-wails",
	}

	outDir := filepath.Join("dist", goos+"_"+goarch)
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	// Build standard binaries.
	for _, pkg := range standardBinaries {
		name := filepath.Base(pkg)
		outFile := filepath.Join(outDir, name)
		if goos == "windows" {
			outFile += ".exe"
		}
		fmt.Printf(">>> building %s/%s -> %s\n", goos, goarch, outFile)
		cmd := exec.Command("go", "build", "-trimpath", "-o", outFile, pkg)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", pkg, err)
		}
	}

	// Build Wails GUI binaries with production flags.
	for _, pkg := range wailsBinaries {
		name := filepath.Base(pkg)
		outFile := filepath.Join(outDir, name)
		if goos == "windows" {
			outFile += ".exe"
		}
		fmt.Printf(">>> building (wails) %s/%s -> %s\n", goos, goarch, outFile)
		ldflags := "-w -s"
		if goos == "windows" {
			ldflags += " -H windowsgui"
		}
		cmd := exec.Command("go", "build",
			"-tags", "desktop,production",
			"-trimpath",
			"-ldflags", ldflags,
			"-o", outFile,
			pkg,
		)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("build %s: %w", pkg, err)
		}
	}

	fmt.Println(">>> build complete. artifacts in", outDir)
	return nil
}

// run executes a command in the current directory, sending output to stdout/stderr.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runInDir executes a command inside dir (relative to repo root).
func runInDir(dir, name string, args ...string) error {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// --------------------------------------------------------------------------
// Gremlins tasks
// --------------------------------------------------------------------------

// Gremlins replaces AI-inserted Unicode gremlin characters with ASCII equivalents
// across all source files in the workspace.
func Gremlins() error {
	return walkAndFix(".", false)
}

// GremlinsDry runs the gremlin replacement in dry-run mode, printing what would change
// without modifying any files.
func GremlinsDry() error {
	return walkAndFix(".", true)
}

func walkAndFix(root string, dryRun bool) error {
	totalFiles := 0
	totalReplacements := 0

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Skip hidden directories and common non-source dirs.
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "vendor" || name == "node_modules" {
				return filepath.SkipDir
			}
			return nil
		}

		if !sourceExtensions[strings.ToLower(filepath.Ext(path))] {
			return nil
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		original := string(raw)
		fixed := original
		fileReplacements := 0

		for _, g := range gremlins {
			count := strings.Count(fixed, g.unicode)
			if count > 0 {
				fileReplacements += count
				if dryRun {
					fmt.Printf("  [dry-run] %s: would replace %d x U+%04X (%s) -> %q\n",
						path, count, []rune(g.unicode)[0], g.name, g.ascii)
				}
				fixed = strings.ReplaceAll(fixed, g.unicode, g.ascii)
			}
		}

		if fileReplacements == 0 {
			return nil
		}

		totalFiles++
		totalReplacements += fileReplacements

		if dryRun {
			fmt.Printf("%s: %d gremlin(s) found\n", path, fileReplacements)
			return nil
		}

		if err := os.WriteFile(path, []byte(fixed), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", path, err)
		}
		fmt.Printf("%s: fixed %d gremlin(s)\n", path, fileReplacements)
		return nil
	})

	if err != nil {
		return err
	}

	if dryRun {
		fmt.Printf("\nDry-run complete: %d file(s) with %d gremlin(s) would be fixed.\n", totalFiles, totalReplacements)
	} else if totalFiles == 0 {
		fmt.Println("No gremlins found.")
	} else {
		fmt.Printf("\nDone: fixed %d gremlin(s) across %d file(s).\n", totalReplacements, totalFiles)
	}
	return nil
}
