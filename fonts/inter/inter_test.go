package inter_test

import (
	"testing"

	"github.com/kuronosu/tela/fonts/inter"
	"github.com/kuronosu/tela/internal/core"
	"github.com/kuronosu/tela/internal/text"
)

// TestInterWinsOverTheSystem: with the system fonts on, Inter is still the
// sans-serif of an app that brings it (the "system" here is a directory of
// test fonts whose family is the desktop's interface font).
func TestInterWinsOverTheSystem(t *testing.T) {
	sys := &text.SystemFontOptions{SkipSystemDirs: true, Dirs: []string{"../../internal/text/testdata/fonts"}, UIFamily: "Noto Sans CJK JP", NoCache: true}
	family := func(c *text.Collection, generic string) string {
		return c.Face(text.Query{Families: []string{generic}}, 16).Typeface().Family()
	}
	plain, err := core.NewFontCollection(nil, nil, sys)
	if err != nil {
		t.Fatal(err)
	}
	if got := family(plain, "sans-serif"); got != "Noto Sans CJK JP" {
		t.Fatalf("the system's sans-serif is %q", got)
	}
	c, err := core.NewFontCollection([]core.Font{core.Font(inter.Sans)}, nil, sys)
	if err != nil {
		t.Fatal(err)
	}
	for _, g := range []string{"sans-serif", "system-ui", "ui-sans-serif", "Inter"} {
		if got := family(c, g); got != "Inter" {
			t.Errorf("%s = %q, want Inter", g, got)
		}
	}
	// The italic file is part of the family.
	f := c.Face(text.Query{Families: []string{"Inter"}, Style: text.StyleItalic}, 16)
	if f == nil || f.Typeface().Style() != text.StyleItalic {
		t.Errorf("no italic Inter: %v", f)
	}
}
