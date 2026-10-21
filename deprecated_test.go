package tela

import "testing"

// TestDeprecatedNamesCompile keeps the names deprecated in phase 7
// (ADR-088) working until phase 8 removes them. It is an internal test so
// that staticcheck's SA1019 does not report the uses.
func TestDeprecatedNamesCompile(t *testing.T) {
	var k Key = KeyEnter
	var c Code = CodeKeyA
	var name KeyName = k // the same types
	var code KeyCode = c
	if name != "Enter" || code != "KeyA" {
		t.Errorf("Key %q, Code %q", name, code)
	}
	var useID func() string = UseId // the same signature as UseID
	_ = useID
}
