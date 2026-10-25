package main

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/telatest"
)

// Frames of the example's layout animations, captured with
// telatest.NoSettle (the clock moves only with Advance). Regenerate the
// goldens with TELA_UPDATE=1 go test ./examples/layout and look at them.

func mount(t *testing.T) *telatest.App {
	return telatest.Mount(t, Layout(), telatest.Size(820, 560), telatest.NoSettle())
}

func TestReorderFrames(t *testing.T) {
	app := mount(t)
	app.Golden("layout.png")
	app.Click(telatest.ByRole("button", "Rotate"))
	app.Advance(50 * time.Millisecond)
	app.Golden("rotate_50ms.png")
	app.Advance(50 * time.Millisecond)
	app.Golden("rotate_100ms.png")
	app.Click(telatest.ByRole("button", "Reverse")) // interrupts the rotation
	app.Advance(80 * time.Millisecond)
	app.Golden("reverse_80ms.png")
	app.Advance(time.Second) // the rows come to rest
	app.ExpectText("Profile the dashboard")
	app.Golden("reversed.png")
}

func TestTabUnderlineFrames(t *testing.T) {
	app := mount(t)
	app.Click(telatest.ByRole("button", "Settings"))
	app.Advance(40 * time.Millisecond)
	app.Golden("tabs_40ms.png")
	app.Advance(50 * time.Millisecond)
	app.Golden("tabs_90ms.png")
	app.Advance(time.Second) // the underline comes to rest
	app.ExpectText("the weekly digest is on")
	app.Golden("tabs_settings.png")
}

func TestExpandFrames(t *testing.T) {
	app := mount(t)
	app.Click(telatest.ByRole("button", "Show details"))
	app.Advance(30 * time.Millisecond)
	app.Golden("expand_30ms.png")
	app.Advance(50 * time.Millisecond)
	app.Golden("expand_80ms.png")
	app.Advance(time.Second) // the card comes to rest
	app.ExpectText("Hide details")
	app.Golden("expanded.png")
	app.Click(telatest.ByRole("button", "Hide details"))
	app.Advance(40 * time.Millisecond)
	app.Golden("collapse_40ms.png")
}
