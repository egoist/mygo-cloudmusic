package app

import (
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

// systemFonts lists the installed font families once, without the
// @-prefixed vertical-writing aliases: fonts installed while the app runs
// show after a restart, as ui.FontFamilies has it.
var systemFonts = sync.OnceValue(func() []string {
	families := ui.FontFamilies()
	names := make([]string, 0, len(families))
	for _, family := range families {
		if family.Name != "" && !strings.HasPrefix(family.Name, "@") {
			names = append(names, family.Name)
		}
	}
	return names
})
