package telatest_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// slidingBox moves and fades a box with a transition when the button is
// clicked.
func slidingBox() tela.Node {
	on, setOn := tela.UseState(false)
	box := "translate-x-0 opacity-100"
	if on {
		box = "translate-x-24 opacity-30"
	}
	return Div(Class("flex flex-col gap-2 p-2"),
		Button(OnClick(func() { setOn.Set(!on) }), Text("Slide")),
		Div(Class("size-10 bg-black transition duration-300 ease-linear", box)))
}

// springBox follows its target with a spring.
func springBox() tela.Node {
	on, setOn := tela.UseState(false)
	target := float32(0)
	if on {
		target = 120
	}
	x := tela.UseSpring(target, tela.Stiffness(200), tela.Damping(20))
	return Div(Class("flex flex-col gap-2 p-2"),
		Button(OnClick(func() { setOn.Set(!on) }), Text("Spring")),
		Div(Class("size-10 bg-black"), Style(tela.TranslateFn(func() (float32, float32) { return x.Get(), 0 }))))
}

func TestFilmCapturesATransition(t *testing.T) {
	app := telatest.Mount(t, tela.C(slidingBox), telatest.Size(240, 100))
	start := app.Now()
	frames := app.Film(450*time.Millisecond, 100*time.Millisecond, func() {
		app.Click(telatest.ByRole("button", "Slide"))
	})
	var ats []time.Duration
	for _, f := range frames {
		ats = append(ats, f.At)
	}
	want := []time.Duration{0, 100, 200, 300, 400, 450}
	if len(ats) != len(want) {
		t.Fatalf("frame times %v, want %v ms", ats, want)
	}
	for i := range want {
		if ats[i] != want[i]*time.Millisecond {
			t.Fatalf("frame times %v, want %v ms", ats, want)
		}
	}
	if got := app.Now() - start; got < 450*time.Millisecond || got > 600*time.Millisecond {
		t.Errorf("the film took %v of app time, want about the span", got)
	}
	for i := 1; i < 3; i++ {
		if bytes.Equal(frames[i-1].Image.Pix, frames[i].Image.Pix) {
			t.Errorf("frames %d and %d are the same picture: the box did not move", i-1, i)
		}
	}
	// The transition lasts 300 ms: the last frame is the settled state.
	settled := app.Screenshot()
	if !bytes.Equal(frames[len(frames)-1].Image.Pix, settled.Pix) {
		t.Error("the last frame is not the settled state")
	}
	if bytes.Equal(frames[0].Image.Pix, settled.Pix) {
		t.Error("the first frame already is the settled state")
	}
}

func TestFilmCapturesASpringAndLeavesTheAppSettling(t *testing.T) {
	app := telatest.Mount(t, tela.C(springBox), telatest.Size(240, 100))
	frames := app.Film(200*time.Millisecond, 50*time.Millisecond, func() {
		app.Click(telatest.ByRole("button", "Spring"))
	})
	if len(frames) != 5 {
		t.Fatalf("%d frames, want 5", len(frames))
	}
	if bytes.Equal(frames[1].Image.Pix, frames[4].Image.Pix) {
		t.Error("the spring did not move between 50 and 200 ms")
	}
	// The span did not reach the end of the spring, and the app is out of
	// NoSettle mode: the next screenshot is the settled one.
	settled := app.Screenshot()
	if bytes.Equal(frames[4].Image.Pix, settled.Pix) {
		t.Error("the spring had settled within 200 ms: pick a slower one")
	}
	if again := app.Screenshot(); !bytes.Equal(settled.Pix, again.Pix) {
		t.Error("a second screenshot differs: the app is still mid-motion")
	}
}

func TestFilmWithoutAnActionFilmsTheMount(t *testing.T) {
	page := func() tela.Node {
		wide, setWide := tela.UseState(false)
		tela.UseMount(func() func() { setWide.Set(true); return nil })
		w := "w-4"
		if wide {
			w = "w-40"
		}
		return Div(Class("p-2"), Div(Class("h-8 bg-black transition-all duration-200 ease-linear", w)))
	}
	app := telatest.Mount(t, tela.C(page), telatest.Size(240, 60), telatest.NoSettle())
	frames := app.Film(200*time.Millisecond, 100*time.Millisecond, nil)
	if len(frames) != 3 {
		t.Fatalf("%d frames, want 3", len(frames))
	}
	if bytes.Equal(frames[0].Image.Pix, frames[1].Image.Pix) || bytes.Equal(frames[1].Image.Pix, frames[2].Image.Pix) {
		t.Error("the entrance of the mount is not in the frames")
	}
}

func TestFilmValidatesItsArguments(t *testing.T) {
	for _, c := range []struct {
		name        string
		span, every time.Duration
		want        string
	}{
		{"zero interval", time.Second, 0, "every must be positive"},
		{"negative span", -time.Second, time.Millisecond, "span cannot be negative"},
		{"too many frames", 10 * time.Second, time.Millisecond, "more than the 100"},
	} {
		t.Run(c.name, func(t *testing.T) {
			mustFail(t, func(tb testing.TB) {
				app := telatest.Mount(tb, tela.C(slidingBox), telatest.Size(100, 60))
				app.Film(c.span, c.every, nil)
			}, c.want)
		})
	}
}
