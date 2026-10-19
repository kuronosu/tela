package core

import (
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

// TestColorScheme: the effective scheme is the app's override or the
// system's preference; a change restyles dark: classes and rebuilds the
// readers of UseColorScheme and UseReducedMotion, and nothing else.
func TestColorScheme(t *testing.T) {
	var schemes []ColorScheme
	var motions []bool
	otherBuilds := 0
	reader := func() Node {
		schemes = append(schemes, UseColorScheme())
		motions = append(motions, UseReducedMotion())
		return div(Key("box"), Class("h-4 bg-white dark:bg-black"))
	}
	other := func() Node {
		otherBuilds++
		return div(Class("h-4"))
	}
	tr := NewTree(Config{Fonts: testFonts, Dark: true})
	tr.SetViewport(gfx.Size{W: 100, H: 100})
	tr.Mount(div(Component(reader), Component(other)))
	tr.Frame()
	bg := func() gfx.Color { return keyed(tr, "box").box.cs.BackgroundColor }
	check := func(when string, want ColorScheme, wantReduced bool, builds int) {
		t.Helper()
		if got := schemes[len(schemes)-1]; got != want || tr.ColorScheme() != want {
			t.Errorf("%s: UseColorScheme %v, Tree %v, want %v", when, got, tr.ColorScheme(), want)
		}
		if got := motions[len(motions)-1]; got != wantReduced {
			t.Errorf("%s: UseReducedMotion %v, want %v", when, got, wantReduced)
		}
		if len(schemes) != builds {
			t.Errorf("%s: %d builds of the reader, want %d", when, len(schemes), builds)
		}
		wantBg := gfx.White
		if want == ColorSchemeDark {
			wantBg = gfx.Black
		}
		if bg() != wantBg {
			t.Errorf("%s: background %v, want %v", when, bg(), wantBg)
		}
	}
	check("system dark", ColorSchemeDark, false, 1)
	tr.SetEnvironment(false, true)
	tr.Frame()
	check("system light, reduced motion", ColorSchemeLight, true, 2)
	tr.SetColorScheme(ColorSchemeDark)
	tr.Frame()
	check("app dark", ColorSchemeDark, true, 3)
	tr.SetEnvironment(false, true) // the system changes nothing now
	tr.Frame()
	check("app dark, system unchanged", ColorSchemeDark, true, 3)
	tr.SetEnvironment(true, true)
	tr.Frame()
	check("app dark over a dark system", ColorSchemeDark, true, 3)
	tr.SetColorScheme(ColorSchemeLight)
	tr.Frame()
	check("app light over a dark system", ColorSchemeLight, true, 4)
	tr.SetColorScheme(ColorSchemeSystem)
	tr.Frame()
	check("system again", ColorSchemeDark, true, 5)
	if otherBuilds != 1 {
		t.Errorf("a component that reads no environment built %d times", otherBuilds)
	}

	// Config.ColorScheme (tela.App.ColorScheme) wins over the system.
	tr2 := NewTree(Config{Fonts: testFonts, Dark: true, ColorScheme: ColorSchemeLight})
	tr2.Mount(Component(reader))
	tr2.Frame()
	if schemes[len(schemes)-1] != ColorSchemeLight {
		t.Errorf("Config.ColorScheme Light over a dark system: %v", schemes[len(schemes)-1])
	}
}

// TestSetColorSchemePosts: the process-wide SetColorScheme reaches the
// tree that receives posts, from any goroutine, and waits for one.
func TestSetColorSchemePosts(t *testing.T) {
	SetColorScheme(ColorSchemeDark) // before any receiver: kept
	tr := NewTree(Config{Fonts: testFonts})
	tr.Mount(div(Class("h-4")))
	stop := tr.ReceivePosts()
	defer stop()
	tr.Frame()
	if tr.ColorScheme() != ColorSchemeDark {
		t.Fatalf("pending SetColorScheme not applied: %v", tr.ColorScheme())
	}
	done := make(chan struct{})
	go func() {
		SetColorScheme(ColorSchemeLight)
		close(done)
	}()
	<-done
	tr.Frame()
	if tr.ColorScheme() != ColorSchemeLight {
		t.Errorf("SetColorScheme from a goroutine: %v", tr.ColorScheme())
	}
	defer func() {
		if recover() == nil {
			t.Error("SetColorScheme(7) did not panic")
		}
	}()
	SetColorScheme(7)
}
