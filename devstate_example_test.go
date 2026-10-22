package tela_test

import (
	"encoding/json"
	"fmt"

	"github.com/kuronosu/tela"
)

// A variable outside the component tree survives a tela dev reload when
// the app saves and restores it. The example plays both processes: in a
// real app tela dev calls Save before it stops the old build and Restore
// before the new one mounts, and the functions live in a file built only
// with the teladev tag (see [tela.DevState]).
func ExampleDevState() {
	selected := "inbox" // lives in a package variable, not in a hook

	devState := tela.DevState{
		Save:    func() ([]byte, error) { return json.Marshal(selected) },
		Restore: func(b []byte) error { return json.Unmarshal(b, &selected) },
	}
	_ = tela.App{Title: "Mail", DevState: devState}

	saved, _ := devState.Save() // the old process, before it stops
	selected = ""               // the new process starts with the zero value...
	devState.Restore(saved)     // ...and gets the state back before it mounts
	fmt.Println(selected)
	// Output: inbox
}
