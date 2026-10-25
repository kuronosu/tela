package kit_test

import (
	"strings"

	"github.com/kuronosu/tela"
)

// The icons the tests draw, from Lucide (ISC, see THIRD_PARTY.md), as
// `tela icons gen` writes them. kit cannot import icons/lucide (a separate
// module), and the components themselves draw none: every icon of a kit
// component is a slot (ADR-129, K7).
var (
	iconMail         = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m22 7-8.991 5.727a2 2 0 0 1-2.009 0L2 7"/><rect x="2" y="4" width="20" height="16" rx="2"/></svg>`}
	iconChevronRight = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="m9 18 6-6-6-6"/></svg>`}
	iconArrowRight   = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M5 12h14"/><path d="m12 5 7 7-7 7"/></svg>`}
	iconCircleAlert  = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/></svg>`}
	iconCheck        = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 6 9 17l-5-5"/></svg>`}
	iconX            = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M18 6 6 18"/><path d="m6 6 12 12"/></svg>`}
)

// svgHTML is how lucide-react renders an icon: the same SVG with its
// "lucide" class, which shadcn's [&_svg:not([class*='size-'])] rules size.
func svgHTML(ic tela.Icon) string {
	return strings.Replace(ic.SVG, "<svg ", `<svg class="lucide" `, 1)
}
