// Package hooks is a fixture of the hooks rule.
package hooks

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/aria"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// A struct component: its Build is the owner of its hooks.
type Counter struct{ Start int }

func (c Counter) Build() tela.Node {
	n, setN := tela.UseState(c.Start)
	hover := aria.UseHover(aria.HoverOptions{})
	_ = kit.UseStyle()
	return Div(hover.Props, OnClick(func() { setN.Set(n + 1) }), Textf("%d", n))
}

// Function components: func() tela.Node and func(P) tela.Node.
func Page() tela.Node {
	tela.UseState(0)
	return Div()
}

func Card(title string) tela.Node {
	tela.UseState(title)
	return Div()
}

// A component made of a function literal.
func Literal() tela.Node {
	return tela.C(func() tela.Node {
		tela.UseState(0)
		return Div()
	})
}

// A custom hook: it calls a hook and is named as one.
func useCounter() int {
	n, _ := tela.UseState(0)
	return n
}

func UseCounter2() (int, func()) {
	n, set := tela.UseState(0)
	return n, func() { set.Set(n + 1) }
}

// useTLS has the name of a hook but calls none: it is not one.
func useTLS() bool { return true }

func Valid(cond bool) tela.Node {
	n := useCounter()
	m, _ := UseCounter2()
	if cond {
		_ = useTLS()
	}
	v := tela.UseValue(0)
	_ = v.Get()
	// A function literal called on the spot is part of its function.
	func() {
		tela.UseState(1)
	}()
	// A nested component has its own state, wherever it is.
	if cond {
		_ = tela.C(func() tela.Node {
			tela.UseState(2)
			return Div()
		})
	}
	// The result of a hook may be used in conditions, after all of them.
	if n > 0 && m > 0 {
		return Div()
	}
	return Div()
}

func Conditional(cond bool) tela.Node {
	items := []int{1, 2}
	if cond {
		tela.UseState(0) // want hooks `hook tela.UseState is called conditionally (inside an if)`
	} else {
		tela.UseState(1) // want hooks `inside an if`
	}
	if cond {
	} else if n, _ := tela.UseState(2); n > 0 { // want hooks `inside an if`
	}
	switch {
	case cond:
		tela.UseRef(0) // want hooks `inside a switch case`
	}
	for range items {
		tela.UseState(0) // want hooks `in a loop`
	}
	for i := 0; i < 3; i++ {
		useCounter() // want hooks `hook useCounter is called in a loop`
	}
	_ = cond && useCounter() > 0 // want hooks `conditionally (after &&)`
	return Div()
}

func EarlyReturn(cond bool) tela.Node {
	a, _ := tela.UseState(0)
	if cond {
		return Div()
	}
	b, _ := tela.UseState(1) // want hooks `after an early return (line`
	_, _ = a, b
	return Div()
}

func Handlers() tela.Node {
	return Div(
		OnClick(func() {
			tela.UseState(0) // want hooks `inside a function literal`
		}),
		Button(OnClick(func() { useCounter() })), // want hooks `inside a function literal`
	)
}

func Effects() tela.Node {
	tela.UseEffect(func() func() {
		tela.UseState(0) // want hooks `inside a function literal`
		return nil
	})
	return Div()
}

func Lists(items []int) tela.Node {
	return Div(
		Map(items, func(i int) tela.Node {
			tela.UseState(i) // want hooks `function given to h.Map`
			return Div(Key(i))
		}),
	)
}

// Not a component and not a hook.
func helper() int {
	n, _ := tela.UseState(0) // want hooks `hook tela.UseState is called from function helper, which is neither a component`
	return n
}

func (c Counter) load() error {
	tela.UseState(0) // want hooks `from method load`
	return nil
}

func Row(a, b int) tela.Node {
	tela.UseState(a + b) // want hooks `from function Row`
	return Div()
}

var global, _ = tela.UseState(0) // want hooks `outside of any function`

func Defer() {
	defer func() {
		tela.UseState(0) // want hooks `inside a function literal`
	}()
}

// More cases that are fine.

type Pointer struct{}

func (p *Pointer) Build() tela.Node {
	tela.UseState(0)
	return Div()
}

func useBox[T any](v T) T {
	x, _ := tela.UseRef(v), 0
	return x.Current
}

func Generic() tela.Node {
	useBox[int](1)
	useBox("s")
	useDrag := func() int {
		n, _ := tela.UseState(0)
		return n
	}
	useDrag()
	switch n, _ := tela.UseState(1); n {
	case 1:
	}
	go func() {}()
	return Div()
}

// More cases that are not.

func useConditionally(cond bool) {
	if cond {
		tela.UseState(0) // want hooks `inside an if`
	}
}

func Selects(ch chan int) tela.Node {
	select {
	case <-ch:
		tela.UseState(0) // want hooks `inside a select case`
	default:
	}
	return Div()
}

func Switches(kind int) tela.Node {
	switch kind {
	case 0:
		return Div()
	}
	tela.UseState(0) // want hooks `after an early return`
	return Div()
}

func Aria(cond bool) tela.Node {
	if cond {
		aria.UsePress(aria.PressOptions{}) // want hooks `hook aria.UsePress is called conditionally`
	}
	go func() {
		tela.UseState(0) // want hooks `inside a function literal`
	}()
	return Div()
}

func Callbacks() tela.Node {
	for range 3 {
	}
	tela.UseState(0)
	return Div()
}

// Components given to an element or a boundary are fine; calling them is not.
func Direct() tela.Node {
	return Div(
		tela.C(Page),
		tela.CP(Card, "x"),
		Counter{Start: 1},
		Valid2(),
		Page(),            // want hooks `Page uses hooks, so it is a component`
		Card("x"),         // want hooks `Card uses hooks`
		Counter{}.Build(), // want hooks `Counter.Build uses hooks and is called directly`
		Plain(),
	)
}

// Valid2 only builds a boundary, and Plain has no hooks: calling them is fine.
func Valid2() tela.Node { return tela.C(Page) }

func Plain() tela.Node { return Div() }
