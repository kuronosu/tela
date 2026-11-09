package tela

import "github.com/kuronosu/tela/internal/core"

// Built-in labels (ADR-251). Tela shows a few texts of its own: the
// fallback title bar's button names, the buttons and default titles of
// message boxes, kit's toast viewport and close button, the standard items
// of menus and the macOS menu bar, the macOS save panel's "Format:". Each
// has a fixed ID and an English text, and one hook translates them all:
// package i18n installs a [LabelSource] that looks the ID up in the app's
// catalogs, so an app translates Tela's texts by adding the keys to its
// catalog files, in the language it follows:
//
//	{"tela": {"messagebox": {"ok": "Aceptar", "cancel": "Cancelar"},
//	          "toast": {"close": "Cerrar aviso"}}}
//
// An ID the catalogs lack keeps its English text (the OS's own on a native
// message box). [Labels] lists the IDs; a kit field such as
// kit.Toaster.CloseLabel overrides the text for one component.

// LabelSource answers a built-in label's ID with the text in the app's
// current language, or false when it has none. args are name/value pairs
// for the {name} placeholders of the English text ("About {name}"). It
// may be called from any goroutine.
type LabelSource = core.LabelSource

// Label returns the text of a built-in label (the IDs [Labels] lists): the
// [LabelSource]'s, else the English text, with the {name} placeholders
// replaced by the args pairs. It does not rebuild components when the
// language changes: use [UseLabels] in components. It panics on an ID
// that is not one of the labels.
//
//	tela.Label("tela.toast.close")                 // "Close toast"
//	tela.Label("tela.menu.quit", "name", "Notes")  // "Quit Notes"
func Label(id string, args ...any) string { return core.Label(id, args...) }

// Labels returns the IDs of the built-in labels with their English texts.
// A {name} in a text is an argument.
func Labels() map[string]string {
	out := map[string]string{}
	for _, id := range core.LabelIDs() {
		out[id], _ = core.LabelDefault(id)
	}
	return out
}

// SetLabelSource sets where the built-in labels' texts come from; nil goes
// back to English. Package i18n sets one when it is imported (the app's
// catalogs, in its current locale), so most apps never call this; an app
// with another translation system does, and calls [LabelsChanged] when its
// language changes. It is safe from any goroutine.
func SetLabelSource(src LabelSource) { core.SetLabelSource(src) }

// LabelsChanged says that the answers of the [LabelSource] changed (the
// language, a catalog): the components that call [UseLabels] build again,
// and the texts the platform shows itself (native message boxes, the macOS
// menus) are given to it again. It is safe from any goroutine.
func LabelsChanged() { core.LabelsChanged() }

// UseLabels returns [Label] and subscribes the component to the source:
// it builds again when [LabelsChanged] runs. It is a hook: call it at the
// top level of a component.
//
//	func CloseButton() tela.Node {
//	    label := tela.UseLabels()
//	    return Button(AriaLabel(label("tela.toast.close")), Text("×"))
//	}
func UseLabels() func(id string, args ...any) string { return core.UseLabels() }
