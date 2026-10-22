//go:build !windows

package main

// checkPrivateDir is only needed on Windows, where newBuildDirIn reuses a
// fixed directory.
func checkPrivateDir(dir string) error { return nil }
