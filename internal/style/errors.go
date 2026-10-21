package style

import (
	"fmt"
	"strings"
)

// UnknownClassError reports a class (or variant) that is not part of the
// vocabulary. Suggestion is the closest valid class by edit distance, empty
// when nothing is close (ARCHITECTURE section 3: errors that teach).
type UnknownClassError struct {
	// Class is the class as written, including its variants.
	Class string
	// Suggestion is the closest valid class, with the same variants.
	Suggestion string
	// Variant is true when the unknown name is a variant (the part before
	// a colon).
	Variant bool
}

func (e *UnknownClassError) Error() string {
	kind := "class"
	if e.Variant {
		kind = "variant"
	}
	if e.Suggestion != "" {
		return fmt.Sprintf("unknown %s %q; did you mean %q?", kind, e.Class, e.Suggestion)
	}
	return fmt.Sprintf("unknown %s %q", kind, e.Class)
}

// UnsupportedClassError reports a class that is valid Tailwind but that
// Tela does not implement (ARCHITECTURE section 10, "Not implemented"),
// with what to use instead.
type UnsupportedClassError struct {
	Class  string
	Reason string
}

func (e *UnsupportedClassError) Error() string {
	return fmt.Sprintf("class %q is not supported: %s", e.Class, e.Reason)
}

// InvalidClassError reports a class whose name is known but whose value is
// not valid, for example an arbitrary value with the wrong type.
type InvalidClassError struct {
	Class      string
	Reason     string
	Suggestion string
}

func (e *InvalidClassError) Error() string {
	msg := fmt.Sprintf("invalid class %q: %s", e.Class, e.Reason)
	if e.Suggestion != "" {
		msg += fmt.Sprintf("; did you mean %q?", e.Suggestion)
	}
	return msg
}

// AliasError reports a problem in the expansion of a class alias
// (NewParserAliases): Alias is the alias written in the class string, Err
// the error of the class of its expansion (an UnknownClassError,
// UnsupportedClassError or InvalidClassError, which errors.As finds).
type AliasError struct {
	Alias string
	Err   error
}

func (e *AliasError) Error() string {
	return fmt.Sprintf("%v (in the class alias %q)", e.Err, e.Alias)
}

// Unwrap returns the error of the expanded class.
func (e *AliasError) Unwrap() error { return e.Err }

// ClassErrors holds every problem found in one class string. The known
// classes of the string are still applied.
type ClassErrors []error

func (e ClassErrors) Error() string {
	parts := make([]string, len(e))
	for i, err := range e {
		parts[i] = err.Error()
	}
	return strings.Join(parts, "; ")
}

// Unwrap returns the individual errors, for errors.As and errors.Is.
func (e ClassErrors) Unwrap() []error { return e }
