//go:build teladev

package i18n

import "github.com/kuronosu/tela/internal/core"

// Under tela dev (ADR-111, ADR-114) the locale chosen with SetLocale is
// kept across reloads. It lives in this package's process-wide store, not
// in hook state, so core's snapshot cannot see it: this registers it with
// core.RegisterDevState, which saves it into the same snapshot and
// restores it before the tree mounts. Only an explicit choice is kept
// (a locale that follows the system needs nothing), so a SetLocale call
// that the edit adds or changes in the app's code wins only until the user
// picks another locale at run time. Compiled only with the teladev build
// tag; release builds do not contain it.
func init() {
	core.RegisterDevState("tela/i18n.locale",
		func() ([]byte, error) {
			if tag := current().explicit; tag != "" {
				return []byte(tag), nil
			}
			return nil, nil
		},
		func(b []byte) error {
			if len(b) > 0 {
				SetLocale(string(b))
			}
			return nil
		})
}
