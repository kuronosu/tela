package core

import "testing"

// TestUseCloseRequest (ADR-253): the close request handlers of a window
// run in mount order with the latest build's function; one that prevents
// the default keeps Options.OnClose from running and the app from
// quitting; without a prevention the default runs; an unmounted handler
// no longer runs.
func TestUseCloseRequest(t *testing.T) {
	var log []string
	onClose := 0
	var setKeep, setShown func(bool)
	handler := func(name string) Node {
		return Component(func() Node {
			keep, set := UseState(true)
			if name == "a" {
				setKeep = set.Set
			}
			UseCloseRequest(func(e CloseRequestEvent) {
				log = append(log, name)
				if keep && name == "a" {
					e.PreventDefault()
					e.Window.Hide()
				}
				if name == "b" && !e.DefaultPrevented() && keep {
					t.Error("b runs after a: a's PreventDefault is seen")
				}
			})
			return nil
		})
	}
	root := Component(func() Node {
		shown, set := UseState(true)
		setShown = set.Set
		return div(handler("a"), If(shown, handler("b")))
	})
	x := newWinHarness(t, Options{Root: root, OnClose: func(WindowHandle) { onClose++ }})
	main := x.windows()[0]
	main.RequestClose()
	x.drain()
	if len(log) != 2 || log[0] != "a" || log[1] != "b" || onClose != 0 || main.Visible() || x.b.Quitting() {
		t.Fatalf("prevented: handlers %v, OnClose %d, visible %v, quitting %v", log, onClose, main.Visible(), x.b.Quitting())
	}
	x.h.Handle(main).Show()
	log = nil
	setShown(false) // b unmounts
	setKeep(false)  // a no longer prevents
	x.drain()
	main.RequestClose()
	x.drain()
	if len(log) != 1 || log[0] != "a" || onClose != 1 {
		t.Errorf("not prevented: handlers %v, OnClose ran %d times; want a only, then OnClose", log, onClose)
	}
}

// TestUseCloseRequestDefaultQuits: without Options.OnClose a close request
// nobody prevents quits, as before.
func TestUseCloseRequestDefaultQuits(t *testing.T) {
	ran := 0
	root := Component(func() Node {
		UseCloseRequest(func(CloseRequestEvent) { ran++ })
		return div(Text("x"))
	})
	x := newWinHarness(t, Options{Root: root})
	x.windows()[0].RequestClose()
	x.drain()
	if ran != 1 || !x.b.Quitting() {
		t.Errorf("handler ran %d times, quitting %v; want once, then the app quits", ran, x.b.Quitting())
	}
}

// TestUseCloseRequestChildWindow: a handler inside a Window component
// answers that window's close request, not the main window's.
func TestUseCloseRequestChildWindow(t *testing.T) {
	childClose, inChild := 0, 0
	root := Component(func() Node {
		return div(win(ChildWindowSpec{WindowSpec: WindowSpec{Title: "C"}, OnClose: func() { childClose++ },
			Root: Component(func() Node {
				UseCloseRequest(func(e CloseRequestEvent) { inChild++; e.PreventDefault() })
				return div(Text("c"))
			})}))
	})
	x := newWinHarness(t, Options{Root: root, OnClose: func(WindowHandle) {}})
	x.child(0).RequestClose()
	x.drain()
	if inChild != 1 || childClose != 0 {
		t.Errorf("child close: handler %d, OnClose %d; want the handler only", inChild, childClose)
	}
	x.windows()[0].RequestClose()
	x.drain()
	if inChild != 1 {
		t.Errorf("the main window's close request ran the child window's handler")
	}
}

// TestUseQuitRequest: a quit request handler that prevents the default
// keeps Options.OnQuit from running; without one OnQuit runs.
func TestUseQuitRequest(t *testing.T) {
	quits, ran := 0, 0
	var setPrevent func(bool)
	root := Component(func() Node {
		prevent, set := UseState(true)
		setPrevent = set.Set
		UseQuitRequest(func(e QuitRequestEvent) {
			ran++
			if prevent {
				e.PreventDefault()
			}
		})
		return div(Text("x"))
	})
	x := newWinHarness(t, Options{Root: root, OnQuit: func() { quits++ }})
	x.b.RequestQuit()
	x.drain()
	if ran != 1 || quits != 0 || x.b.Quitting() {
		t.Fatalf("prevented: handler %d, OnQuit %d, quitting %v", ran, quits, x.b.Quitting())
	}
	setPrevent(false)
	x.drain()
	x.b.RequestQuit()
	x.drain()
	if ran != 2 || quits != 1 {
		t.Errorf("not prevented: handler %d, OnQuit %d; want OnQuit after the handler", ran, quits)
	}
}
