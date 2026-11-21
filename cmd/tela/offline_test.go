package main

import (
	"os"
	"testing"
)

// offlineUnresolved ends a test whose module outside the repository cannot
// resolve its requirements from the module cache (the tests run with
// GOPROXY=off). Locally that skips: a fresh cache lacks the test-only
// modules of tela's dependencies. On GitHub Actions every test job warms
// the cache first, so there it fails: a skip would hide the end-to-end
// tests of dev, mcp, new and build.
func offlineUnresolved(t *testing.T, format string, args ...any) {
	t.Helper()
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		t.Fatalf(format+" (the job's module cache was warmed, so this is a failure on CI)", args...)
	}
	t.Skipf(format, args...)
}
