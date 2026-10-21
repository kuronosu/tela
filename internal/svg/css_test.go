package svg

import (
	"strings"
	"testing"
)

func TestStyleSheetCascade(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"type", `<style>rect{fill:#0000ff}</style><rect width="1" height="1"/>`, "#0000ff"},
		{"class over type", `<style>.a{fill:#00ff00} rect{fill:#0000ff}</style><rect class="x a" width="1" height="1"/>`, "#00ff00"},
		{"id over class", `<style>#i{fill:#ff0000} .a.b{fill:#00ff00}</style><rect id="i" class="a b" width="1" height="1"/>`, "#ff0000"},
		{"compound", `<style>circle.a{fill:#ff0000} rect.a{fill:#00ff00}</style><rect class="a" width="1" height="1"/>`, "#00ff00"},
		{"source order", `<style>.a{fill:#ff0000}</style><style>.a{fill:#00ff00}</style><rect class="a" width="1" height="1"/>`, "#00ff00"},
		{"list", `<style>circle, .a , #z{fill:#00ff00}</style><rect class="a" width="1" height="1"/>`, "#00ff00"},
		{"universal", `<style>*{fill:#00ff00}</style><rect width="1" height="1"/>`, "#00ff00"},
		{"sheet over attribute", `<style>rect{fill:#00ff00}</style><rect fill="#ff0000" width="1" height="1"/>`, "#00ff00"},
		{"style attribute over sheet", `<style>#i{fill:#ff0000}</style><rect id="i" style="fill:#00ff00" width="1" height="1"/>`, "#00ff00"},
		{"important sheet over style attribute", `<style>rect{fill:#00ff00!important}</style><rect style="fill:#ff0000" width="1" height="1"/>`, "#00ff00"},
		{"important style attribute last", `<style>rect{fill:#ff0000 !important}</style><rect style="fill:#00ff00 !important" width="1" height="1"/>`, "#00ff00"},
		{"inherited from group", `<style>.g{fill:#00ff00}</style><g class="g"><rect width="1" height="1"/></g>`, "#00ff00"},
		{"invalid value keeps earlier", `<style>rect{fill:#00ff00} .a{fill:bogus}</style><rect class="a" width="1" height="1"/>`, "#00ff00"},
		{"descendant skipped", `<style>g rect{fill:#ff0000} rect{fill:#00ff00}</style><g><rect width="1" height="1"/></g>`, "#00ff00"},
		{"list with unsupported keeps the rest", `<style>g > rect, .a{fill:#00ff00}</style><rect class="a" width="1" height="1"/>`, "#00ff00"},
		{"syntax error drops the rule", `<style>rect{fill:#00ff00} .a..b, rect{fill:#ff0000}</style><rect class="a" width="1" height="1"/>`, "#00ff00"},
		{"at rules skipped", `<style>@import url(x.css); @media screen { rect{fill:#ff0000} } rect{fill:#00ff00}</style><rect width="1" height="1"/>`, "#00ff00"},
		{"comments and cdata", `<style><![CDATA[ /* rect{fill:#ff0000} */ rect{fill:#00ff00} ]]></style><rect width="1" height="1"/>`, "#00ff00"},
		{"other type ignored", `<style type="text/less">rect{fill:#ff0000}</style><rect fill="#00ff00" width="1" height="1"/>`, "#00ff00"},
		{"unknown property ignored", `<style>rect{transform:scale(2);fill:#00ff00}</style><rect width="1" height="1"/>`, "#00ff00"},
		{"unclosed block ends with its sheet", `<style>rect{fill:#ff0000</style><style>rect{fill:#00ff00}</style><rect width="1" height="1"/>`, "#00ff00"},
		{"case-sensitive tag", `<style>RECT{fill:#ff0000}</style><rect fill="#00ff00" width="1" height="1"/>`, "#00ff00"},
		{"use instance", `<style>.u{fill:#00ff00}</style><defs><rect id="r" class="u" width="1" height="1"/></defs><use href="#r"/>`, "#00ff00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dump(t, svgDoc(c.body), Options{})
			if strings.Count(got, "FillPath") != 1 || !strings.Contains(got, "paint="+c.want) {
				t.Errorf("want one fill %s, got\n%s", c.want, got)
			}
		})
	}
}

func TestStyleSheetStroke(t *testing.T) {
	got := dump(t, svgDoc(`<style>.icon{fill:none;stroke:#000000;stroke-width:3;stroke-linecap:round}</style><path class="icon" d="M0 0L10 10"/>`), Options{})
	if !strings.Contains(got, "width=3") || !strings.Contains(got, "cap=round") || strings.Contains(got, "FillPath") {
		t.Errorf("stroke from the sheet:\n%s", got)
	}
}

func TestSwitch(t *testing.T) {
	cases := []struct {
		name, body, want string
	}{
		{"first", `<switch><rect fill="#00ff00" width="1" height="1"/><rect fill="#ff0000" width="1" height="1"/></switch>`, "#00ff00"},
		{"language", `<switch><rect systemLanguage="fr" fill="#ff0000" width="1" height="1"/><rect systemLanguage="de, en-GB" fill="#00ff00" width="1" height="1"/></switch>`, "#00ff00"},
		{"extensions fail", `<switch><rect requiredExtensions="" fill="#ff0000" width="1" height="1"/><rect fill="#00ff00" width="1" height="1"/></switch>`, "#00ff00"},
		{"unknown feature fails", `<switch><rect requiredFeatures="http://www.w3.org/TR/SVG11/feature#Font" fill="#ff0000" width="1" height="1"/><rect requiredFeatures="http://www.w3.org/TR/SVG11/feature#Shape" fill="#00ff00" width="1" height="1"/></switch>`, "#00ff00"},
		{"title skipped", `<switch><title>t</title><desc>d</desc><rect fill="#00ff00" width="1" height="1"/></switch>`, "#00ff00"},
		{"inherits from switch", `<switch fill="#00ff00"><g><rect width="1" height="1"/></g></switch>`, "#00ff00"},
		{"nested", `<switch><switch><rect systemLanguage="ja" fill="#ff0000" width="1" height="1"/><use href="#r"/></switch></switch><defs><rect id="r" fill="#00ff00" width="1" height="1"/></defs>`, "#00ff00"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := dump(t, svgDoc(c.body), Options{})
			if strings.Count(got, "FillPath") != 1 || !strings.Contains(got, "paint="+c.want) {
				t.Errorf("want one fill %s, got\n%s", c.want, got)
			}
		})
	}
	// The chosen child may render nothing; the next ones are not tried.
	if got := dump(t, svgDoc(`<switch><text>x</text><rect fill="#ff0000" width="1" height="1"/></switch>`), Options{}); got != "" {
		t.Errorf("an unsupported first child renders nothing:\n%s", got)
	}
	if got := dump(t, svgDoc(`<switch><rect systemLanguage="fr" fill="#ff0000" width="1" height="1"/></switch>`), Options{}); got != "" {
		t.Errorf("no passing child renders nothing:\n%s", got)
	}
}

func TestStyleSheetLimits(t *testing.T) {
	var sb strings.Builder
	sb.WriteString(`<svg xmlns="http://www.w3.org/2000/svg"><style>`)
	for i := 0; i < 5000; i++ {
		sb.WriteString(".a{fill:#000000}")
	}
	sb.WriteString(`</style>`)
	for i := 0; i < 2000; i++ {
		sb.WriteString(`<rect class="a" width="1" height="1"/>`)
	}
	sb.WriteString(`</svg>`)
	if _, err := Parse([]byte(sb.String())); err != ErrTooComplex {
		t.Errorf("4096 rules on 2000 elements: err = %v, want ErrTooComplex", err)
	}
}
