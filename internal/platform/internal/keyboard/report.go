package keyboard

import (
	"flag"
	"fmt"
	"os"
)

// Report prints a keyboard setup problem (a keymap or Compose file that
// cannot be used) on standard error as "tela: <err>", except in a go test
// binary, and returns err for the backend to record: the keyboard keeps
// working with physical codes only (or without dead keys), and a silent
// failure would be impossible to diagnose.
func Report(err error) error {
	if !underTest() {
		fmt.Fprintln(os.Stderr, "tela:", err)
	}
	return err
}

// underTest reports whether the process is a go test binary.
func underTest() bool { return flag.Lookup("test.v") != nil }
