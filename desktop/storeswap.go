package desktop

import "errors"

// unsupportedSwap is an error of an atomic exchange or exclusive rename
// that the caller handles as errors.ErrUnsupported, falling back to a
// plain rename (storeswap_*.go).
type unsupportedSwap struct{ err error }

func (e unsupportedSwap) Error() string { return "atomic exchange unsupported: " + e.err.Error() }

func (e unsupportedSwap) Is(target error) bool { return target == errors.ErrUnsupported }

func (e unsupportedSwap) Unwrap() error { return e.err }
