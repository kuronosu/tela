//go:build !linux && !darwin

package desktop

import "errors"

// exchangeFiles is unsupported: Windows has no atomic exchange of two
// files (ReplaceFileW is a sequence of renames, with the same gap), and
// the other Unix systems are not handled.
func exchangeFiles(from, to string) error { return errors.ErrUnsupported }

// renameNoReplace is unsupported, as exchangeFiles.
func renameNoReplace(from, to string) error { return errors.ErrUnsupported }
