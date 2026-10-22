//go:build teladev

package main

import (
	"encoding/json"
	"fmt"

	"github.com/kuronosu/tela"
)

// Only tela dev links this: a release build of the app has neither the
// save and restore code nor encoding/json for it.
func init() {
	devState = tela.DevState{
		Save: func() ([]byte, error) { return json.Marshal(pkgCount) },
		Restore: func(b []byte) error {
			fmt.Printf("devapp: restore %s\n", b)
			return json.Unmarshal(b, &pkgCount)
		},
	}
}
