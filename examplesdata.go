package examplesdata

import "embed"

//go:embed examples/*.PLD examples/*.jed
var FS embed.FS
