package tela_test

import (
	"context"
	"fmt"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

func ExampleMain() {
	counter := func() tela.Node {
		count, setCount := tela.UseState(0)
		return Button(OnClick(func(tela.MouseEvent) { setCount.Set(count + 1) }), Textf("Clicked %d times", count))
	}
	// In package main. Main returns when the window is closed; without a
	// display it prints the error and exits with status 1.
	tela.Main(tela.App{Title: "Demo", Width: 400, Height: 300, Root: counter})
}

func ExampleUseMount() {
	t := &exampleT{}
	defer t.close()
	var events []string
	panel := func() tela.Node {
		tela.UseMount(func() func() {
			events = append(events, "subscribe")
			return func() { events = append(events, "unsubscribe") }
		})
		return Text("Panel")
	}
	page := func() tela.Node {
		open, setOpen := tela.UseState(true)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setOpen.Set(!open) }), Text("Toggle")),
			If(open, tela.C(panel)),
		)
	}
	app := telatest.Mount(t, tela.C(page))
	app.Click(telatest.ByRole("button", "Toggle")) // unmounts the panel
	app.Click(telatest.ByRole("button", "Toggle")) // mounts it again
	fmt.Println(events)
	// Output: [subscribe unsubscribe subscribe]
}

func ExampleUseOnce() {
	t := &exampleT{}
	defer t.close()
	created := 0
	type cache struct{ hits map[string]int }
	page := func() tela.Node {
		c := tela.UseOnce(func() *cache { created++; return &cache{hits: map[string]int{}} })
		n, setN := tela.UseState(0)
		c.hits["build"]++
		return Button(OnClick(func(tela.MouseEvent) { setN.Set(n + 1) }), Textf("Add %d", n))
	}
	app := telatest.Mount(t, tela.C(page))
	app.Click(telatest.ByText("Add"))
	app.Click(telatest.ByText("Add"))
	fmt.Println("created", created, "times")
	// Output: created 1 times
}

func ExampleUseEffectDo() {
	t := &exampleT{}
	defer t.close()
	var title string
	inbox := func() tela.Node {
		unread, setUnread := tela.UseState(3)
		tela.UseEffectDo(func() { title = fmt.Sprintf("Inbox (%d)", unread) }, unread)
		return Button(OnClick(func(tela.MouseEvent) { setUnread.Set(0) }), Text("Mark all read"))
	}
	app := telatest.Mount(t, tela.C(inbox))
	fmt.Println(title)
	app.Click(telatest.ByRole("button", "Mark all read"))
	fmt.Println(title)
	// Output:
	// Inbox (3)
	// Inbox (0)
}

func ExampleUseUpdateEffect() {
	t := &exampleT{}
	defer t.close()
	var saved []string
	editor := func() tela.Node {
		theme, setTheme := tela.UseState("light")
		tela.UseUpdateEffect(func() func() {
			saved = append(saved, theme) // not on mount: only changes are saved
			return nil
		}, theme)
		return Button(OnClick(func(tela.MouseEvent) { setTheme.Set("dark") }), Text("Dark"))
	}
	app := telatest.Mount(t, tela.C(editor))
	fmt.Println(saved)
	app.Click(telatest.ByRole("button", "Dark"))
	fmt.Println(saved)
	// Output:
	// []
	// [dark]
}

func ExampleUseInterval() {
	t := &exampleT{}
	defer t.close()
	timer := func() tela.Node {
		seconds, setSeconds := tela.UseState(0)
		running, setRunning := tela.UseState(true)
		period := time.Second
		if !running {
			period = 0 // paused
		}
		tela.UseInterval(period, func() { setSeconds.Set(seconds + 1) })
		return Div(
			P(Textf("%d s", seconds)),
			Button(OnClick(func(tela.MouseEvent) { setRunning.Set(!running) }), Text("Pause")),
		)
	}
	app := telatest.Mount(t, tela.C(timer))
	app.Advance(3 * time.Second) // the interval runs on the app's clock
	app.ExpectText("3 s")
	app.Click(telatest.ByRole("button", "Pause"))
	app.Advance(5 * time.Second)
	app.ExpectText("3 s")
	fmt.Println("ok")
	// Output: ok
}

func ExampleUseGo() {
	t := &exampleT{}
	defer t.close()
	search := func(ctx context.Context, q string) []string {
		return []string{q + " one", q + " two"} // a slow call that honors ctx
	}
	results := func() tela.Node {
		query, setQuery := tela.UseState("tela")
		found, setFound := tela.UseState([]string(nil))
		tela.UseGo(func(ctx context.Context) {
			r := search(ctx, query)
			if ctx.Err() == nil { // a newer query cancelled this one
				setFound.Set(r) // safe from any goroutine
			}
		}, query)
		return Div(
			Button(OnClick(func(tela.MouseEvent) { setQuery.Set("go") }), Text("Search go")),
			Map(found, func(s string) tela.Node { return P(Key(s), Text(s)) }),
		)
	}
	app := telatest.Mount(t, tela.C(results))
	app.ExpectText("tela two") // expectations wait for the goroutine
	app.Click(telatest.ByRole("button", "Search go"))
	app.ExpectText("go one")
	fmt.Println("ok")
	// Output: ok
}

func ExampleSetter_IsZero() {
	t := &exampleT{}
	defer t.close()
	type tabsCtl struct{ SetTab tela.Setter[int] }
	tabs := tela.CreateContext(tabsCtl{}) // without a Provider, SetTab is zero
	tab := func() tela.Node {
		ctl := tela.UseContext(tabs)
		return Button(OnClick(func(tela.MouseEvent) {
			if !ctl.SetTab.IsZero() { // Set on a zero Setter panics
				ctl.SetTab.Set(1)
			}
		}), Text("Second"))
	}
	app := telatest.Mount(t, tela.C(tab))
	app.Click(telatest.ByRole("button", "Second"))
	fmt.Println(tela.Setter[int]{}.IsZero())
	// Output: true
}

// dialogProps are the props of closableDialog: OnClose is optional.
type dialogProps struct {
	Title   string
	OnClose tela.Callback[func()]
}

func closableDialog(p dialogProps) tela.Node {
	return Div(H2(Text(p.Title)),
		If(!p.OnClose.IsZero(), Button(OnClick(func(tela.MouseEvent) { p.OnClose.Fn()() }), Text("Close"))))
}

func ExampleCallback_IsZero() {
	t := &exampleT{}
	defer t.close()
	app := telatest.Mount(t, tela.CP(closableDialog, dialogProps{Title: "Saved"}))
	fmt.Println(app.Count(telatest.ByRole("button", "Close")))
	// Output: 0
}
