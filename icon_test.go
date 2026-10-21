package tela_test

import (
	"strings"
	"testing"

	"github.com/kuronosu/tela"
)

func TestParseSVG(t *testing.T) {
	src := `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24"><path d="M4 12h16"/></svg>`
	ic, err := tela.ParseSVG([]byte(src))
	if err != nil || ic.SVG != src {
		t.Fatalf("ParseSVG = %q, %v", ic.SVG, err)
	}
	if ic != (tela.Icon{SVG: src}) {
		t.Error("an Icon is compared by its text")
	}
	for _, bad := range []string{"", "<g/>", "<svg><path d='M0 0'></svg"} {
		if ic, err := tela.ParseSVG([]byte(bad)); err == nil || ic != (tela.Icon{}) {
			t.Errorf("ParseSVG(%q) = %v, %v; want an error", bad, ic, err)
		}
	}
	defer func() {
		if r := recover(); r == nil || !strings.Contains(r.(string), "tela.MustSVG") {
			t.Errorf("MustSVG of a bad document: recover() = %v", r)
		}
	}()
	tela.MustSVG([]byte("<g/>"))
}
