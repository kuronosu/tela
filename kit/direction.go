package kit

import (
	"fmt"
	"strings"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// checkDirection returns the direction a Direction field names, in lower
// case: "ltr" or "rtl". Another value panics in dev mode and is "ltr"
// otherwise, as aria's hooks do.
func checkDirection(component, d string) string {
	switch l := strings.ToLower(d); l {
	case "ltr", "rtl":
		return l
	default:
		if devMode() {
			panic(fmt.Sprintf("kit: %s.Direction %q is not a direction: use \"ltr\" or \"rtl\"", component, d))
		}
		return "ltr"
	}
}

// useDirection is the direction of a component, a hook: its Direction field
// when given, else the direction at its position (tela.UseDirection: the
// nearest h.Dir above it, "ltr" without one). rootDir is the h.Dir prop the
// component adds to its root when its own field set the direction, so that
// its layout follows what its keys do (nil otherwise).
func useDirection(component, field string) (dir string, rootDir tela.Child) {
	at := tela.UseDirection()
	if field == "" {
		return at, nil
	}
	dir = checkDirection(component, field)
	return dir, h.Dir(dir)
}
