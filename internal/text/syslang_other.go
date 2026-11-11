//go:build !windows

package text

// platformLanguage has no operating-system source outside Windows: the
// language comes from the environment (syslang.go).
func platformLanguage() string { return "" }
