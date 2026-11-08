package core

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/kuronosu/tela/internal/assets/fonts/inter"
	"github.com/kuronosu/tela/internal/assets/fonts/jetbrainsmono"
	"github.com/kuronosu/tela/internal/platform"
	"github.com/kuronosu/tela/internal/platform/headless"
	"github.com/kuronosu/tela/internal/text"
)

// The desktop's interface and monospace fonts (fonts.go, ADR-244): a
// Host gives the platform's families to a collection with system fonts
// before the first frame, and shapes every text again when they change.

// fakeSystemFonts returns a collection whose "installed" fonts are Inter
// and JetBrains Mono, from a temporary directory (never the machine's).
func fakeSystemFonts(t *testing.T) *text.Collection {
	t.Helper()
	dir := t.TempDir()
	for name, data := range map[string][]byte{"Inter.ttf": inter.Upright, "JetBrainsMono.ttf": jetbrainsmono.Upright} {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	c, err := text.NewSystemCollection(text.SystemFontOptions{Dirs: []string{dir}, SkipSystemDirs: true})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// desktopFontsScreen is text in the sans and mono stacks, in paragraphs
// and in an input.
func desktopFontsScreen() Node {
	return div(Class("flex flex-col items-start gap-2 p-2"),
		p(Key("sans"), Class("font-sans"), Text("iiii")),
		p(Key("mono"), Class("font-mono"), Text("iiii")),
		input(Key("in"), Class("font-sans"), DefaultValue("iiii")),
	)
}

// desktopFontsHost starts desktopFontsScreen on a headless backend with
// settings s and the fonts given.
func desktopFontsHost(t *testing.T, s platform.SystemSettings, fonts *text.Collection) *winHarness {
	t.Helper()
	b := headless.New(headless.Options{Size: platform.Size{W: 200, H: 150}, Settings: s})
	h, err := NewHost(b, Options{Root: desktopFontsScreen(), Fonts: fonts})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(h.stopTimer)
	x := &winHarness{t: t, b: b, h: h}
	x.drain()
	return x
}

func TestDesktopFontsFollowTheSettings(t *testing.T) {
	x := desktopFontsHost(t, platform.SystemSettings{UIFontFamily: "JetBrains Mono"}, fakeSystemFonts(t))
	tr := x.h.Tree()
	width := func(k string) float32 { return tr.elemBounds(keyed(tr, k)).Width() }
	field := func() float32 { return keyed(tr, "in").box.fld.layout().MaxContentWidth() }

	// JetBrains Mono's i is 9.6px wide at 16px, Inter's much narrower;
	// without a desktop family the stacks fall back to Inter (no family
	// of the operating system's lists is installed).
	const mono = 4 * 9.6
	if w := width("sans"); w != mono {
		t.Errorf("font-sans with JetBrains Mono as the interface font: width %g, want %g", w, mono)
	}
	if w := field(); w != mono {
		t.Errorf("the input's text with JetBrains Mono: width %g, want %g", w, mono)
	}
	narrow := width("mono")
	if narrow >= mono {
		t.Fatalf("font-mono without a monospace family: width %g, want Inter's (< %g)", narrow, mono)
	}

	// A change of the families shapes the text again.
	after := platform.SystemSettings{MonoFontFamily: "JetBrains Mono"}
	x.b.SetSettings(after)
	x.drain()
	if w := width("sans"); w != narrow {
		t.Errorf("font-sans after the change: width %g, want %g", w, narrow)
	}
	if w := field(); w != narrow {
		t.Errorf("the input after the change: width %g, want %g", w, narrow)
	}
	if w := width("mono"); w != mono {
		t.Errorf("font-mono after the change: width %g, want %g", w, mono)
	}
	// The window shows what an app started with those settings shows.
	fresh := desktopFontsHost(t, after, fakeSystemFonts(t))
	if got, want := x.b.Windows()[0].Frame(), fresh.b.Windows()[0].Frame(); !bytes.Equal(got.Pix, want.Pix) {
		t.Error("the frame after the change differs from a fresh start's")
	}

	// Another setting leaves the text as it is.
	shapes := tr.Stats().Shapes
	x.b.SetSettings(platform.SystemSettings{MonoFontFamily: "JetBrains Mono", ReducedMotion: true})
	x.drain()
	if n := tr.Stats().Shapes; n != shapes {
		t.Errorf("a settings change without new fonts shaped %d texts", n-shapes)
	}
}

// TestDesktopFontsIgnoredWithoutSystemFonts checks that the deterministic
// set of the tests does not follow the desktop's fonts.
func TestDesktopFontsIgnoredWithoutSystemFonts(t *testing.T) {
	x := desktopFontsHost(t, platform.SystemSettings{UIFontFamily: "JetBrains Mono"}, testFonts)
	tr := x.h.Tree()
	f := testFonts.Face(text.Query{Families: []string{"ui-sans-serif", "system-ui", "sans-serif"}}, 16)
	if f == nil || f.Typeface().Family() != "Inter" {
		t.Fatalf("the test fonts draw font-sans with %v, want Inter", f)
	}
	before := tr.elemBounds(keyed(tr, "sans")).Width()
	shapes := tr.Stats().Shapes
	x.b.SetSettings(platform.SystemSettings{UIFontFamily: "Inter", MonoFontFamily: "Inter"})
	x.drain()
	if w := tr.elemBounds(keyed(tr, "sans")).Width(); w != before {
		t.Errorf("width %g after the change, want %g", w, before)
	}
	if n := tr.Stats().Shapes; n != shapes {
		t.Errorf("the change shaped %d texts", n-shapes)
	}
}
