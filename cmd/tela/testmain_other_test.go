//go:build !darwin

package main

import (
	"os"
	"testing"
)

// serveTests runs the tests and exits with their status. On macOS the main
// thread also serves AppKit calls meanwhile (testmain_darwin_test.go).
func serveTests(m *testing.M) {
	os.Exit(m.Run())
}
