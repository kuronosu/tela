//go:build !linux && !darwin

package update

import "errors"

// exchange is unsupported: Windows has no atomic exchange of two folders,
// and the other Unix systems are not handled. The caller uses two renames.
func exchange(a, b string) error { return errors.ErrUnsupported }
