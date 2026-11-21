// Package migapi is the current API of a package whose old spellings the
// tests of the migrate rule rewrite (migrate_test.go).
package migapi

// Widget was {Label} with a Display method.
type Widget struct{ Title string }

// Show was Display.
func (w *Widget) Show() {}

// Build was Make.
func Build() *Widget { return &Widget{} }

// Hello is what the package at the old path, migold, had.
func Hello() string { return "hello" }
