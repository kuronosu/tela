package h

import "testing"

// TestDeprecatedIdCompiles keeps h.Id, deprecated in phase 7 for ID
// (ADR-088), working until phase 8 removes it. An internal test, so that
// staticcheck's SA1019 does not report the use.
func TestDeprecatedIdCompiles(t *testing.T) {
	if n := Div(Label(For("email"), Text("Email")), Input(Id("email"))); n.IsZero() {
		t.Error("a Div with an Id is the zero node")
	}
}
