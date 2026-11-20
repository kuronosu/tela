package devtools

import "github.com/kuronosu/tela"

// The icons of the DevTools are Lucide's (ISC; THIRD_PARTY.md), copied
// unaltered from icons/lucide, which the root module cannot import, as
// literals; the record and stop marks are drawn here.
func lucide(paths string) tela.Icon {
	return tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round">` + paths + `</svg>`}
}

var (
	iconChevronRight = lucide(`<path d="m9 18 6-6-6-6"/>`)
	iconChevronDown  = lucide(`<path d="m6 9 6 6 6-6"/>`)
	iconSearch       = lucide(`<path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/>`)
	iconCrosshair    = lucide(`<circle cx="12" cy="12" r="10"/><line x1="22" x2="18" y1="12" y2="12"/><line x1="6" x2="2" y1="12" y2="12"/><line x1="12" x2="12" y1="6" y2="2"/><line x1="12" x2="12" y1="22" y2="18"/>`)
	iconTrash        = lucide(`<path d="M10 11v6"/><path d="M14 11v6"/><path d="M19 6v14a2 2 0 0 1-2 2H7a2 2 0 0 1-2-2V6"/><path d="M3 6h18"/><path d="M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2"/>`)
	iconFoldAll      = lucide(`<path d="m7 20 5-5 5 5"/><path d="m7 4 5 5 5-5"/>`)
	iconUnfoldAll    = lucide(`<path d="m7 15 5 5 5-5"/><path d="m7 9 5-5 5 5"/>`)
	iconExternal     = lucide(`<path d="M15 3h6v6"/><path d="M10 14 21 3"/><path d="M18 13v6a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V8a2 2 0 0 1 2-2h6"/>`)
	iconLayers       = lucide(`<path d="M12.83 2.18a2 2 0 0 0-1.66 0L2.6 6.08a1 1 0 0 0 0 1.83l8.58 3.91a2 2 0 0 0 1.66 0l8.58-3.9a1 1 0 0 0 0-1.83z"/><path d="M2 12a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 12"/><path d="M2 17a1 1 0 0 0 .58.91l8.6 3.91a2 2 0 0 0 1.65 0l8.58-3.9A1 1 0 0 0 22 17"/>`)

	// The marks of the record button: a disc and a square, filled.
	iconRecord = lucide(`<circle cx="12" cy="12" r="6" fill="currentColor"/>`)
	iconStop   = lucide(`<rect x="6" y="6" width="12" height="12" rx="1" fill="currentColor"/>`)
)
