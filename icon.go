package tela

import (
	"fmt"

	"github.com/kuronosu/tela/internal/core"
)

// Icon is an SVG icon, shown with h.Icon. Its SVG field holds the text of
// an SVG document, so that icons can be package-level data:
//
//	var Check = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 24 24" fill="none"
//	  stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">
//	  <path d="M20 6 9 17l-5-5"/></svg>`}
//
// The document is parsed the first time an icon with that text is drawn
// and the result is cached by the text for the life of the process (in a
// cache bounded by size), so drawing it again, at any size or color,
// never parses it again. Icon is comparable. ParseSVG and MustSVG check
// the text when the icon is created.
//
// Icons are drawn as vectors at the device scale, with currentColor as
// the element's text color; see h.Icon.
type Icon struct {
	SVG string
}

// ParseSVG parses data as an SVG document and returns it as an Icon. The
// parsed document is cached, so the first draw does not parse it again.
// It returns the parse error of malformed XML, a root that is not svg,
// or a document beyond Tela's limits on size.
func ParseSVG(data []byte) (Icon, error) {
	ic := Icon{SVG: string(data)}
	if _, err := core.ParseIcon(ic.SVG); err != nil {
		return Icon{}, err
	}
	return ic, nil
}

// MustSVG is ParseSVG for documents known to be valid, such as embedded
// files: it panics on error.
//
//	//go:embed logo.svg
//	var logoSVG []byte
//	var Logo = tela.MustSVG(logoSVG)
func MustSVG(data []byte) Icon {
	ic, err := ParseSVG(data)
	if err != nil {
		panic(fmt.Sprintf("tela.MustSVG: %v", err))
	}
	return ic
}
