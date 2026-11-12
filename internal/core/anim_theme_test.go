package core

import (
	"testing"
	"time"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// A theme, color scheme or system dark mode change applies without
// transitions by default (ADR-276); Config.ThemeTransitions keeps them.

const themeBox = "size-10 bg-[#ffffff] dark:bg-[#000000] transition-colors duration-200 ease-linear"

func newInputCfg(t *testing.T, root Node, cfg Config) *inputHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 200, H: 200}})
	cfg.Fonts, cfg.Flavor = testFonts, testFlavor
	host, err := NewHost(b, Options{Config: cfg, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	t.Cleanup(host.stopTimer)
	return &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
}

func (x *inputHarness) noAnim(what string) {
	x.t.Helper()
	if ba := x.cs("b").anim; ba != nil && !ba.empty() {
		x.t.Errorf("%s: the box animates: %+v", what, ba)
	}
	if x.tree().Animating() {
		x.t.Errorf("%s: the tree has running animations", what)
	}
}

func TestSchemeChangeIsInstant(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(themeBox))), 100, 100)
	x.settle()
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	if x.bg("b") != gfx.Black {
		t.Errorf("dark on the next frame: %v", x.bg("b"))
	}
	x.noAnim("SetColorScheme")
	x.tree().SetColorScheme(ColorSchemeLight)
	x.frame()
	if x.bg("b") != gfx.White {
		t.Errorf("light on the next frame: %v", x.bg("b"))
	}
	x.noAnim("SetColorScheme back")
}

func TestSystemDarkChangeIsInstant(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(themeBox))), 100, 100)
	x.settle()
	x.b.SetSettings(platform.SystemSettings{DarkMode: true})
	x.b.Drain(x.h)
	x.frame()
	if x.bg("b") != gfx.Black {
		t.Errorf("dark on the next frame: %v", x.bg("b"))
	}
	x.noAnim("system dark mode")
}

func TestThemeChangeIsInstant(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("size-10 bg-primary transition-colors duration-200 ease-linear"))), 100, 100)
	x.settle()
	th := DefaultTheme()
	th.Colors["primary"] = SchemeColor{Light: gfx.RGBA8(255, 0, 0, 255), Dark: gfx.RGBA8(0, 0, 255, 255)}
	x.tree().SetTheme(th)
	x.frame()
	if c := x.bg("b"); c != gfx.RGBA8(255, 0, 0, 255) {
		t.Errorf("the new primary on the next frame: %v", c)
	}
	x.noAnim("SetTheme")
}

func TestThemeTransitionsKeepsTheFade(t *testing.T) {
	x := newInputCfg(t, div(div(Key("b"), Class(themeBox))), Config{ThemeTransitions: true})
	x.settle()
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no transition with ThemeTransitions: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 100*time.Millisecond)
	if c := x.bg("b"); !near32(c.R, 0.5, 1e-4) {
		t.Errorf("half way to dark: %v", c)
	}
	x.settle()
	if x.bg("b") != gfx.Black {
		t.Errorf("dark: %v", x.bg("b"))
	}
	// A system change fades too.
	x.tree().SetColorScheme(ColorSchemeSystem)
	x.settle()
	x.b.SetSettings(platform.SystemSettings{DarkMode: true})
	x.b.Drain(x.h)
	x.frame()
	if ba := x.cs("b").anim; ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no transition for the system change with ThemeTransitions: %+v", ba)
	}
}

const hoverTheme = "size-10 bg-[#ffffff] hover:bg-[#ff0000] dark:bg-[#000000] dark:hover:bg-[#0000ff] transition-colors duration-200 ease-linear"

// A hover after the switch fades as usual.
func TestHoverAfterSchemeChangeTransitions(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(hoverTheme))), 200, 200)
	x.settle()
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	x.noAnim("switch")
	x.send(move(20, 20))
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("the hover after the switch does not transition: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 100*time.Millisecond)
	if c := x.bg("b"); c.B == 0 || c.B == 1 || c.R != 0 {
		t.Errorf("mid-way to the dark hover color: %v", c)
	}
	x.settle()
	if x.bg("b") != gfx.RGBA8(0, 0, 255, 255) {
		t.Errorf("dark hover: %v", x.bg("b"))
	}
}

// A transition running when the switch lands jumps to its end with the
// new tokens.
func TestRunningTransitionJumpsOnSchemeChange(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(hoverTheme))), 200, 200)
	x.settle()
	x.send(move(20, 20))
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no hover transition: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 50*time.Millisecond)
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	if x.bg("b") != gfx.RGBA8(0, 0, 255, 255) {
		t.Errorf("the running transition did not jump to the dark hover color: %v", x.bg("b"))
	}
	x.noAnim("jump")
}

// A transition running on a property the switch does not change keeps
// going.
func TestUnrelatedTransitionSurvivesSchemeChange(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("size-10 bg-[#ffffff] hover:bg-[#ff0000] dark:text-white transition-colors duration-200 ease-linear"))), 200, 200)
	x.settle()
	x.send(move(20, 20))
	x.frame()
	ba := x.cs("b").anim
	if ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no hover transition: %+v", ba)
	}
	x.advanceTo(ba.trans[0].Start + 50*time.Millisecond)
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	if ba := x.cs("b").anim; ba == nil || len(ba.trans) == 0 {
		t.Fatalf("the background transition was cancelled: %+v", ba)
	}
	x.settle()
	if x.bg("b") != gfx.RGBA8(255, 0, 0, 255) {
		t.Errorf("final: %v", x.bg("b"))
	}
}

// The Scheme prop of a subtree is an ordinary change of the app's tree,
// not a theme change: it transitions (ADR-276).
func TestSchemePropChangeStillTransitions(t *testing.T) {
	var set Setter[ColorScheme]
	app := func() Node {
		s, st := UseState(ColorSchemeLight)
		set = st
		return div(div(Key("panel"), SchemeProp(s), div(Key("b"), Class(themeBox))))
	}
	x := newInput(t, Component(app), 200, 200)
	x.settle()
	set.Set(ColorSchemeDark)
	x.frame()
	if ba := x.cs("b").anim; ba == nil || len(ba.trans) != 1 {
		t.Fatalf("no transition after the Scheme prop changed: %+v", ba)
	}
}

// Keyframe animations are not transitions: they keep running through a
// switch.
func TestKeyframesSurviveSchemeChange(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class("size-10 animate-spin dark:bg-black"))), 100, 100)
	x.runFor(50 * time.Millisecond)
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	if ba := x.cs("b").anim; ba == nil || ba.kf == nil {
		t.Fatalf("the keyframe animation stopped: %+v", ba)
	}
	if !x.tree().Animating() {
		t.Error("the tree stopped animating")
	}
}

// The instant window ends with the frame: later changes transition.
func TestInstantEndsWithTheFrame(t *testing.T) {
	x := newInput(t, div(div(Key("b"), Class(themeBox))), 100, 100)
	x.settle()
	x.tree().SetColorScheme(ColorSchemeDark)
	x.frame()
	if x.tree().anim.instant || x.tree().anim.instantSeen {
		t.Error("the instant window stayed open after the frame")
	}
}
