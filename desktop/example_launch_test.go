//go:build !windows

package desktop_test

import (
	"fmt"

	"github.com/kuronosu/tela/desktop"
)

// The files a launch names, made absolute.
func ExampleLaunch() {
	l := desktop.Launch{Dir: "/home/ana", Args: []string{"--new-window", "notes:open?id=3", "file:///tmp/a%20b.note", "lists/groceries.note"}}
	fmt.Println(l.URLs())
	fmt.Println(l.Files())
	// Output:
	// [notes:open?id=3]
	// [/tmp/a b.note /home/ana/lists/groceries.note]
}
