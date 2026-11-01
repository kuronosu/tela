package tela_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/telatest"
)

// settingsScreen shows what the settings hooks read, a box sized in rem
// and a box with a contrast-more: border width.
func settingsScreen() tela.Node {
	hc, ts := tela.UseHighContrast(), tela.UseTextScale()
	accent, ok := tela.UseAccentColor()
	return Div(Class("flex flex-col items-start"),
		P(Textf("hc=%v ts=%g accent=%v %v", hc, ts, accent, ok)),
		Div(TestID("rem"), Class("w-16 h-4")),
		Div(TestID("px"), Class("w-[100px] h-[10px]")),
		Div(TestID("border"), Class("border contrast-more:border-4"), Div(Class("size-[10px]"))),
	)
}

// The system settings v2 reach the hooks and the classes: high contrast
// (contrast-more:), the text scale (rem, unless App.IgnoreTextScale) and
// the accent color; tests use the defaults unless they set them.
func TestSystemSettingsV2(t *testing.T) {
	for _, tc := range []struct {
		name     string
		opts     []telatest.Option
		text     string
		rem, px  float32
		borderPx float32
	}{
		{"defaults", nil, "hc=false ts=1 accent=#00000000 false", 64, 100, 12},
		{"high contrast", []telatest.Option{telatest.HighContrast()}, "hc=true ts=1 accent=#00000000 false", 64, 100, 18},
		{"text scale", []telatest.Option{telatest.TextScale(1.5)}, "hc=false ts=1.5 accent=#00000000 false", 96, 100, 12},
		{"text scale ignored", []telatest.Option{telatest.TextScale(1.5), telatest.FromApp(tela.App{IgnoreTextScale: true})},
			"hc=false ts=1 accent=#00000000 false", 64, 100, 12},
		{"accent color", []telatest.Option{telatest.AccentColor(tela.Hex("#3584e4"))}, "hc=false ts=1 accent=#3584e4 true", 64, 100, 12},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := telatest.Mount(t, tela.C(settingsScreen), tc.opts...)
			app.ExpectText(tc.text)
			if r := app.Bounds(telatest.ByTestID("rem")); r.Width != tc.rem {
				t.Errorf("w-16: %g wide, want %g", r.Width, tc.rem)
			}
			if r := app.Bounds(telatest.ByTestID("px")); r.Width != tc.px {
				t.Errorf("w-[100px]: %g wide, want %g", r.Width, tc.px)
			}
			if r := app.Bounds(telatest.ByTestID("border")); r.Width != tc.borderPx {
				t.Errorf("the bordered box: %g wide, want %g (its 10 px content and the borders)", r.Width, tc.borderPx)
			}
		})
	}
}

// A text scale outside (0, 4] is a mistake in the test.
func TestTextScaleOptionChecked(t *testing.T) {
	for _, s := range []float64{-1, 5} {
		ft := &fakeTB{TB: t}
		func() {
			defer func() { recover() }()
			telatest.Mount(ft, tela.C(settingsScreen), telatest.TextScale(s))
		}()
		if !ft.failed {
			t.Errorf("TextScale(%g) did not fail the test", s)
		}
	}
}

// fakeTB records a fatal error instead of ending the test.
type fakeTB struct {
	testing.TB
	failed bool
}

func (f *fakeTB) Fatalf(format string, args ...any) {
	f.failed = true
	panic(fmt.Sprintf(format, args...))
}
