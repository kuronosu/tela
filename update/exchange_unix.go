//go:build linux || darwin

package update

import "errors"

// unsupportedExchange is an exchange error the caller handles as
// errors.ErrUnsupported, falling back to two renames (exchange_*.go).
type unsupportedExchange struct{ err error }

func (e unsupportedExchange) Error() string { return "atomic exchange unsupported: " + e.err.Error() }

func (e unsupportedExchange) Is(target error) bool { return target == errors.ErrUnsupported }

func (e unsupportedExchange) Unwrap() error { return e.err }
