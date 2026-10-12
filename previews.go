package tela

import "sort"

// Previews are the named screens of a package, for `tela snapshot` and
// `tela inspect` (ARCHITECTURE section 17). Define one as a package-level
// variable called Previews:
//
//	var Previews = tela.Previews{
//	    "Login":    LoginScreen,
//	    "Settings": SettingsScreen,
//	}
//
// Each function builds a screen as a component, so it may use hooks. The
// CLI renders the preview selected with --preview NAME; without the flag it
// renders the first name in byte order (see [Previews.Names]), so a name
// such as "1 Login" can pick the default. A package with a single screen
// can define `func Preview() tela.Node` instead.
type Previews map[string]func() Node

// Names returns the preview names in the order the CLI uses: sorted by
// bytes. The first is the default preview.
func (p Previews) Names() []string {
	names := make([]string, 0, len(p))
	for name := range p {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
