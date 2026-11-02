package core

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
)

// The platform flavor of a Tree (Config.Flavor, ADR-221): the key
// bindings and word rules of its text fields and the CmdOrCtrl of its
// events follow it, whatever the OS the test runs on.

// newFlavorKeys mounts root in a focused headless window whose Tree has
// flavor fl.
func newFlavorKeys(t *testing.T, root Node, fl platform.Flavor) *inputHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 400, H: 200}})
	host, err := NewHost(b, Options{Config: Config{Fonts: testFonts, Flavor: fl}, Root: root})
	if err != nil {
		t.Fatal(err)
	}
	b.Drain(host)
	x := &inputHarness{t: t, b: b, h: host, w: host.Window().(*headless.Window)}
	x.w.SetFocused(true)
	x.b.Drain(x.h)
	return x
}

// chord presses a letter with modifiers, with its text as a backend
// reports it (the text ignores Ctrl; the field decides whether it types).
func (x *inputHarness) chord(letter string, mods platform.Modifiers) {
	c, _ := platform.ParseKeyCode("Key" + strings.ToUpper(letter))
	k := platform.Key(letter)
	if mods&platform.ModShift != 0 {
		k = platform.Key(strings.ToUpper(letter))
	}
	x.keys(platform.KeyEvent{Action: platform.KeyPress, Code: c, Key: k, Text: string(k), Mods: mods},
		platform.KeyEvent{Action: platform.KeyRelease, Code: c, Key: k, Mods: mods})
}

// expectSel fails when the selection of field k is not [s, e).
func (x *inputHarness) expectSel(what string, k any, s, e int) {
	x.t.Helper()
	if gs, ge := x.selection(k); gs != s || ge != e {
		x.t.Errorf("%s: selection %d..%d, want %d..%d", what, gs, ge, s, e)
	}
}

func TestTreeFlavorResolved(t *testing.T) {
	if got := NewTree(Config{}).Flavor(); got != platform.HostFlavor() {
		t.Errorf("zero Config.Flavor gives %v, want the host's %v", got, platform.HostFlavor())
	}
	for _, fl := range []platform.Flavor{platform.FlavorLinux, platform.FlavorWindows, platform.FlavorMac} {
		if got := NewTree(Config{Flavor: fl}).Flavor(); got != fl {
			t.Errorf("Config.Flavor %v gives %v", fl, got)
		}
	}
}

// The Mac bindings run end to end through a Tree on any OS: Command for
// select all, undo and redo, Option by word, Command to the line edges,
// and Control+A is Emacs's line start, not select all.
func TestFieldMacFlavor(t *testing.T) {
	root := div(Class("p-4"), input(Key("in"), DefaultValue("one two three")))
	x := newFlavorKeys(t, root, platform.FlavorMac)
	x.frame()
	x.tab()
	x.expectSel("Tab", "in", 0, 13)
	x.key(platform.KeyArrowRight, platform.ModSuper)
	x.expectSel("Cmd+Right", "in", 13, 13)
	x.key(platform.KeyArrowLeft, platform.ModAlt)
	x.expectSel("Option+Left", "in", 8, 8)
	x.key(platform.KeyArrowLeft, platform.ModAlt)
	x.expectSel("Option+Left twice", "in", 4, 4)
	x.key(platform.KeyArrowRight, platform.ModAlt)
	x.expectSel("Option+Right", "in", 7, 7)
	x.key(platform.KeyArrowLeft, platform.ModSuper)
	x.expectSel("Cmd+Left", "in", 0, 0)
	x.chord("a", platform.ModSuper)
	x.expectSel("Cmd+A", "in", 0, 13)
	x.chord("a", platform.ModCtrl)
	x.expectSel("Ctrl+A (line start, not select all)", "in", 0, 0)
	if v := x.value("in"); v != "one two three" {
		t.Fatalf("Ctrl+A typed: %q", v)
	}

	x.key(platform.KeyArrowRight, platform.ModSuper)
	x.typeText("X")
	if v := x.value("in"); v != "one two threeX" {
		t.Fatalf("typing: %q", v)
	}
	x.chord("z", platform.ModCtrl)
	if v := x.value("in"); v != "one two threeX" {
		t.Errorf("Ctrl+Z undid under the Mac flavor: %q", v)
	}
	x.chord("z", platform.ModSuper)
	if v := x.value("in"); v != "one two three" {
		t.Errorf("Cmd+Z: %q", v)
	}
	x.chord("z", platform.ModSuper|platform.ModShift)
	if v := x.value("in"); v != "one two threeX" {
		t.Errorf("Cmd+Shift+Z: %q", v)
	}
	// Option types characters on a Mac (Option+E is a dead key in the US
	// layout; a layout that gives text gets it typed).
	x.keys(platform.KeyEvent{Action: platform.KeyPress, Code: platform.CodeUnidentified, Key: "å", Text: "å", Mods: platform.ModAlt},
		platform.KeyEvent{Action: platform.KeyRelease, Code: platform.CodeUnidentified, Key: "å", Mods: platform.ModAlt})
	if v := x.value("in"); v != "one two threeXå" {
		t.Errorf("Option+a: %q", v)
	}
}

// The Linux and Windows bindings through a Tree: Control shortcuts (Meta
// does nothing), the word rules (Windows goes to the start of the next
// word) and Up/Down in a single-line field (to its edges on Linux,
// nothing on Windows).
func TestFieldLinuxWindowsFlavor(t *testing.T) {
	for _, c := range []struct {
		fl               platform.Flavor
		wordRight        int // Ctrl+Right from 0 in "one two three"
		down, up         int // the caret after Down from 0, then Up from the end
		ctrlDeleteResult string
	}{
		{platform.FlavorLinux, 3, 13, 0, " two three"},
		{platform.FlavorWindows, 4, 0, 13, "two three"},
	} {
		t.Run(c.fl.String(), func(t *testing.T) {
			root := div(Class("p-4"), input(Key("in"), DefaultValue("one two three")))
			x := newFlavorKeys(t, root, c.fl)
			x.frame()
			x.tab()
			x.key(platform.KeyHome, 0)
			x.expectSel("Home", "in", 0, 0)
			x.key(platform.KeyArrowRight, platform.ModCtrl)
			x.expectSel("Ctrl+Right", "in", c.wordRight, c.wordRight)
			x.key(platform.KeyHome, 0)
			x.key(platform.KeyArrowDown, 0)
			x.expectSel("Down", "in", c.down, c.down)
			x.key(platform.KeyEnd, 0)
			x.key(platform.KeyArrowUp, 0)
			x.expectSel("Up", "in", c.up, c.up)

			x.chord("a", platform.ModSuper)
			x.key(platform.KeyEnd, 0)
			x.expectSel("Meta+A then End", "in", 13, 13)
			x.chord("a", platform.ModSuper)
			x.expectSel("Meta+A (not select all)", "in", 13, 13)
			x.chord("a", platform.ModCtrl)
			x.expectSel("Ctrl+A", "in", 0, 13)
			x.key(platform.KeyHome, 0)
			x.key(platform.KeyDelete, platform.ModCtrl)
			if v := x.value("in"); v != c.ctrlDeleteResult {
				t.Errorf("Ctrl+Delete: %q, want %q", v, c.ctrlDeleteResult)
			}
			x.chord("z", platform.ModSuper)
			if v := x.value("in"); v != c.ctrlDeleteResult {
				t.Errorf("Meta+Z undid: %q", v)
			}
			x.chord("z", platform.ModCtrl)
			if v := x.value("in"); v != "one two three" {
				t.Errorf("Ctrl+Z: %q", v)
			}
		})
	}
}

// CmdOrCtrl reports Meta under the Mac flavor and Ctrl under the others,
// on every event type with modifiers, following the Tree that dispatched
// the event.
func TestCmdOrCtrlPerFlavor(t *testing.T) {
	for _, fl := range []platform.Flavor{platform.FlavorLinux, platform.FlavorWindows, platform.FlavorMac} {
		t.Run(fl.String(), func(t *testing.T) {
			var log []string
			add := func(kind string, ctrl, meta, cmd bool) {
				log = append(log, fmt.Sprintf("%s ctrl=%v meta=%v cmd=%v", kind, ctrl, meta, cmd))
			}
			root := div(Key("box"), Class("size-[200px] overflow-auto"), TabIndex(0),
				On("keydown", func(e KeyboardEvent) { add("key", e.Ctrl, e.Meta, e.CmdOrCtrl()) }),
				On("pointerdown", func(e PointerEvent) { add("pointer", e.Ctrl, e.Meta, e.CmdOrCtrl()) }),
				On("mousedown", func(e MouseEvent) { add("mouse", e.Ctrl, e.Meta, e.CmdOrCtrl()) }),
				On("wheel", func(e WheelEvent) { add("wheel", e.Ctrl, e.Meta, e.CmdOrCtrl()) }),
				On("dragstart", func(e DragEvent) { add("drag", e.Ctrl, e.Meta, e.CmdOrCtrl()) }),
				div(Class("h-[1000px]")))
			x := newFlavorKeys(t, root, fl)
			x.frame()
			x.tab()
			for _, mods := range []platform.Modifiers{platform.ModCtrl, platform.ModSuper} {
				log = log[:0]
				x.chord("s", mods)
				held := platform.ButtonPrimary.Mask()
				for _, e := range []platform.PointerEvent{
					ptr(platform.PointerMove, 50, 50, platform.ButtonNone, 0),
					ptr(platform.PointerDown, 50, 50, platform.ButtonPrimary, held),
					ptr(platform.PointerMove, 70, 50, platform.ButtonNone, held),
					ptr(platform.PointerUp, 70, 50, platform.ButtonPrimary, 0),
				} {
					e.Mods = mods
					x.send(e)
				}
				x.wheel(platform.ScrollEvent{Pos: platform.Point{X: 50, Y: 50}, Steps: platform.Point{Y: 1}, Mods: mods})
				ctrl, meta := mods == platform.ModCtrl, mods == platform.ModSuper
				cmd := ctrl
				if fl == platform.FlavorMac {
					cmd = meta
				}
				want := ""
				for _, k := range []string{"key", "pointer", "mouse", "drag", "wheel"} {
					want += fmt.Sprintf("%s ctrl=%v meta=%v cmd=%v\n", k, ctrl, meta, cmd)
				}
				if got := strings.Join(log, "\n") + "\n"; got != want {
					t.Errorf("mods %v:\n%s\nwant\n%s", mods, got, want)
				}
			}
		})
	}
}

// A hand-built event, which no Tree dispatched, follows the host.
func TestCmdOrCtrlHandBuilt(t *testing.T) {
	mac := platform.HostFlavor() == platform.FlavorMac
	for _, c := range []struct {
		name string
		got  func(ctrl, meta bool) bool
	}{
		{"KeyboardEvent", func(c, m bool) bool { return KeyboardEvent{Ctrl: c, Meta: m}.CmdOrCtrl() }},
		{"MouseEvent", func(c, m bool) bool { return MouseEvent{Ctrl: c, Meta: m}.CmdOrCtrl() }},
		{"PointerEvent", func(c, m bool) bool { return PointerEvent{Ctrl: c, Meta: m}.CmdOrCtrl() }},
		{"WheelEvent", func(c, m bool) bool { return WheelEvent{Ctrl: c, Meta: m}.CmdOrCtrl() }},
		{"DragEvent", func(c, m bool) bool { return DragEvent{Ctrl: c, Meta: m}.CmdOrCtrl() }},
	} {
		if got := c.got(true, false); got != !mac {
			t.Errorf("%s{Ctrl}.CmdOrCtrl() = %v on %v", c.name, got, platform.HostFlavor())
		}
		if got := c.got(false, true); got != mac {
			t.Errorf("%s{Meta}.CmdOrCtrl() = %v on %v", c.name, got, platform.HostFlavor())
		}
		if c.got(false, false) {
			t.Errorf("%s{}.CmdOrCtrl() = true", c.name)
		}
	}
	// The events stay comparable (map keys compile only for comparable
	// types).
	_ = map[KeyboardEvent]bool{{Key: "s", Ctrl: true}: true}
	_ = map[MouseEvent]bool{{Ctrl: true}: true}
	_ = map[PointerEvent]bool{{Ctrl: true}: true}
	_ = map[WheelEvent]bool{{Ctrl: true}: true}
	_ = map[DragEvent]bool{{Ctrl: true}: true}
}

// Child windows follow the main Tree's flavor.
func TestChildConfigFlavor(t *testing.T) {
	tr := NewTree(Config{Flavor: platform.FlavorMac})
	if got := tr.childConfig("", 1).Flavor; got != platform.FlavorMac {
		t.Errorf("child window flavor %v, want mac", got)
	}
}
