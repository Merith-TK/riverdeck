// Package resources embeds static assets that are shared across binaries.
package resources

import _ "embed"

// IconSVG is the Riverdeck application icon, embedded at compile time.
//go:embed icon.svg
var IconSVG []byte
