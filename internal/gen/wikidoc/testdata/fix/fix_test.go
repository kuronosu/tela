package fix_test

import (
	"fmt"

	"example.com/fix/fix"
)

func Example() {
	w := fix.NewWidget("a")
	fmt.Println(w.Name)
	// Output: a
}

func ExampleNewWidget() {
	w := fix.NewWidget("b")
	fmt.Println(w.Mode == fix.ModeFast)
	// Output: true
}

func ExampleWidget_Draw() {
	// Draw reports how long it took.
	fmt.Println(fix.NewWidget("c").Draw())
	// Output:
	// 0s
}
