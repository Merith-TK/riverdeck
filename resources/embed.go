// Package resources embeds static assets that are shared across binaries.
package resources

import (
	"embed"
	"io/fs"
)

// IconSVG is the Riverdeck application icon, embedded at compile time.
//
//go:embed icon.svg
var IconSVG []byte

// IconPNG is a 64x64 PNG icon suitable for system tray use.
//
//go:embed icons/icon_64.png
var IconPNG []byte

// editorFiles contains the static files for the layout editor web UI.
//
//go:embed editor
var editorFiles embed.FS

// packageFiles contains the bundled default packages (extracted to configDir on first run).
//
//go:embed packages
var packageFiles embed.FS

// EditorAssetsFS returns an fs.FS rooted at the editor/ directory for use
// with the Wails v2 AssetServer.
func EditorAssetsFS() fs.FS {
	sub, err := fs.Sub(editorFiles, "editor")
	if err != nil {
		panic("resources: failed to sub editor assets FS: " + err.Error())
	}
	return sub
}

// DefaultPackagesFS returns a sub-filesystem rooted at the "packages/"
// directory.  The caller can use fs.WalkDir to enumerate and extract files.
func DefaultPackagesFS() fs.FS {
	sub, err := fs.Sub(packageFiles, "packages")
	if err != nil {
		panic("resources: failed to sub packages FS: " + err.Error())
	}
	return sub
}
