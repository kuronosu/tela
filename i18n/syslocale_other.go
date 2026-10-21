//go:build !windows

package i18n

// platformLocale has no operating-system source outside Windows: the
// locale comes from the environment (syslocale.go).
func platformLocale() string { return "" }
