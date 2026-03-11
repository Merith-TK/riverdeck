//go:build mage

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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
