package i18n

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// Tela's own texts (ADR-251). Importing i18n installs the catalogs as the
// source of the built-in labels (tela.SetLabelSource): a catalog message
// whose key is the label's ID, such as "tela.toast.close", translates it.

func init() {
	tela.SetLabelSource(func(id string, args ...any) (string, bool) {
		return Formatter{}.lookup(id, args)
	})
}

// Label returns the text of one of Tela's built-in labels (the IDs
// [tela.Labels] lists, such as "tela.toast.close" or "tela.menu.quit") for
// the Formatter's locale: the message of that key in the locale's
// catalogs, their parents' and the fallback's, else Tela's English text.
// args are name/value pairs for the label's placeholders ("name" in
// "About {name}"). A missing key is not reported: Tela's labels are
// optional to translate. It panics on an ID that is not one of the labels.
//
// Components get the Formatter of their locale from [UseFormatter], which
// follows a [Provider]; [tela.Label] and [tela.UseLabels] use the current
// locale for the places that have no tree (the title bar, native boxes).
func (f Formatter) Label(id string, args ...any) string {
	english := core.LabelEnglish(id, args...) // checks the ID
	if s, ok := f.lookup(id, args); ok {
		return s
	}
	return english
}
