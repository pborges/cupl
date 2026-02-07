package cupl

import (
	_ "embed"
	"strings"
)

//go:embed VERSION
var versionRaw string

// Version returns the embedded version string from VERSION.
func Version() string {
	return strings.TrimSpace(versionRaw)
}
