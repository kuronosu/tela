package win32

import (
	"strings"

	"github.com/kuronosu/tela/internal/platform/assoc"
)

// stripToastFlags returns args without the flags COM adds when it starts
// the app for a toast clicked after the app exited (ADR-297; the owner,
// 2026-11-17): assoc.ToastActivatorFlag, which the LocalServer32 command
// and the MSIX manifest give, and "-Embedding" (or "/Embedding"), which
// COM appends to a local server's command line. "-Embedding" is removed
// only when the activator's flag is there, so a program that is a COM
// server of its own keeps it. activated reports that the flag was there.
// args[0], the program, is kept.
func stripToastFlags(args []string) (out []string, activated bool) {
	if len(args) < 2 {
		return args, false
	}
	for _, a := range args[1:] {
		if a == assoc.ToastActivatorFlag {
			activated = true
			break
		}
	}
	if !activated {
		return args, false
	}
	out = make([]string, 0, len(args))
	out = append(out, args[0])
	for _, a := range args[1:] {
		if a == assoc.ToastActivatorFlag || strings.EqualFold(a, "-Embedding") || strings.EqualFold(a, "/Embedding") {
			continue
		}
		out = append(out, a)
	}
	return out, true
}
