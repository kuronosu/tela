package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
)

// Changes of the environment (dark mode, the window size crossing a
// breakpoint) restyle boxes like any other change, so they transition.

func TestDarkModeChangeTransitions(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("size-10 bg-[#ffffff] dark:bg-[#000000] transition-colors duration-200 ease-linear"))), 100, 100)
	x.settle()
	if x.bg("b") != gfx.White {
		t.Fatalf("light: %v", x.bg("b"))
	}
	x.b.SetSettings(platform.SystemSettings{DarkMode: true})
	x.b.Drain(x.h)
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no transition after the dark mode change: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 100*time.Millisecond)
	if c := x.bg("b"); !near32(c.R, 0.5, 1e-4) {
		t.Errorf("half way to dark: %v", c)
	}
	x.settle()
	if x.bg("b") != gfx.Black {
		t.Errorf("dark: %v", x.bg("b"))
	}
}

func TestBreakpointChangeTransitions(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("h-10 w-20 md:w-40 bg-red-500 transition-[width] duration-200 ease-linear"))), 400, 100)
	x.settle()
	if w := x.cs("b").ld.Layout().Size.W; w != 80 {
		t.Fatalf("below md: width %v", w)
	}
	x.w.Resize(platform.Size{W: 800, H: 100}) // md is 768 px
	x.b.Drain(x.h)
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no transition after the breakpoint changed: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 100*time.Millisecond)
	if w := x.cs("b").ld.Layout().Size.W; !near32(w, 120, 0.01) {
		t.Errorf("half way to md:w-40: %v", w)
	}
	x.settle()
	if w := x.cs("b").ld.Layout().Size.W; w != 160 {
		t.Errorf("at md: %v", w)
	}
}
