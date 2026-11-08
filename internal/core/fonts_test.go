package core

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// TestDefaultFontsRegistered checks that a tree without Config.Fonts uses
// the registered default set (internal/stdfonts, which the tests of this
// package link), not the system's fonts.
func TestDefaultFontsRegistered(t *testing.T) {
	c, err := defaultFonts()
	if err != nil || c == nil {
		t.Fatalf("defaultFonts: %v, %v", c, err)
	}
	if got := c.Families(); len(got) == 0 || got[0] != "Inter" {
		t.Fatalf("default families %v, want the standard set", got)
	}
	tr := NewTree(Config{})
	if f := tr.fonts.Face(text.Query{Families: []string{"ui-sans-serif", "system-ui", "sans-serif"}}, 16); f == nil || f.Typeface().Family() != "Inter" {
		t.Fatalf("a tree without fonts draws the sans stack with %v, want Inter", f)
	}
}

// TestRunReportsNoFont checks what Run reports for an app whose text finds
// no font (a container without fonts): the reason the system fonts are
// off, and the package to import, once.
func TestRunReportsNoFont(t *testing.T) {
	var buf bytes.Buffer
	c := text.NewCollection()
	reportFonts(c, errors.New("no font directory"), &buf)
	for range 3 {
		text.NewLayout(c, &text.Paragraph{Text: "Hello", Default: text.TextStyle{Families: []string{"system-ui"}, Size: 16}})
	}
	out := buf.String()
	if !strings.Contains(out, "tela: system fonts disabled: no font directory") {
		t.Errorf("output %q lacks the reason", out)
	}
	if strings.Count(out, NoFontMessage) != 1 || !strings.Contains(NoFontMessage, "github.com/kuronosu/tela/fonts/inter") || !strings.Contains(NoFontMessage, "tela.App.Fonts") {
		t.Errorf("output %q: want the no-font message once", out)
	}
	// Font files that cannot be parsed are reported too.
	buf.Reset()
	c = text.NewCollection()
	if err := c.AddSource(&text.FontSource{Family: "Broken", Data: [][]byte{[]byte("junk")}}); err != nil {
		t.Fatal(err)
	}
	reportFonts(c, nil, &buf)
	c.Face(text.Query{Families: []string{"Broken"}}, 16)
	if out := buf.String(); !strings.Contains(out, `tela: text: font family "Broken"`) || !strings.Contains(out, NoFontMessage) {
		t.Errorf("output %q", out)
	}
}
