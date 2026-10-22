//go:build !teladev

package core

import (
	"io/fs"

	"github.com/kuronosu/tela/internal/platform"
)

// Without the teladev build tag the child runtime of tela dev (devrun.go,
// ADR-114) is not compiled: these stubs are only called behind
// devStateEnabled, which is false, so the compiler removes the calls.
// Release builds do not change (ADR-111).

func devRecover()                                  {}
func devBackend(Options) (platform.Backend, error) { return nil, nil }
func devRun(platform.Backend, *Host) error         { return nil }
func devAfterFrame(*Host)                          {}
func devAssetAdded(fs.FS)                          {}
