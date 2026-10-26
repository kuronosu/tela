// Package lookalike has functions with the names of Tela's, which the
// linter must not mistake for them.
package lookalike

type Prop struct{}

func Class(parts ...string) Prop { return Prop{} }
func Role(role string) Prop      { return Prop{} }
func AriaSort(s string) Prop     { return Prop{} }
func Dir(s string) Prop          { return Prop{} }
func UseState() int              { return 0 }

type Button struct{ Class string }

func Use() {
	_ = Class("not-a-class rounded-xxl")
	_ = Role("buton")
	_ = AriaSort("up")
	_ = Dir("sideways")
	_ = Button{Class: "rounded-xxl"}
	if true {
		_ = UseState()
	}
}
