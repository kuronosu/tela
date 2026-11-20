package core

import (
	"strings"
	"testing"
)

// TestInspectFontFamily checks that the family of StyleSummary.Font is the
// one that drew the box's text, not the first name of its font-family
// stack: font-sans starts with ui-sans-serif, which the deterministic
// fonts resolve to Inter.
func TestInspectFontFamily(t *testing.T) {
	root := div(
		div(Key("sans"), Class("font-sans"), Text("Hello")),
		div(Key("mono"), Class("font-mono"), Text("Hello")),
		div(Key("serif"), Class("font-serif"), Text("Hello")),
		// The family that drew the most glyphs wins.
		p(Key("mostly-sans"), Class("font-sans"), Text("a long sentence with "), code(Class("font-mono"), Text("x"))),
		p(Key("mostly-mono"), Class("font-sans"), Text("a "), code(Class("font-mono"), Text("long_identifier_name"))),
		// Fallback: Inter has no Arabic, the per-script family draws it.
		div(Key("arabic"), Class("font-sans"), Text("مرحبا بالعالم")),
		// No text: the first family of the stack that has a font.
		div(Key("empty-serif"), Class("font-serif h-4")),
		div(Key("block-mono"), Class("font-mono"), div(Class("h-4"))),
		// A text field reports the family of its displayed text.
		input(Key("field"), Class("font-mono"), DefaultValue("value")),
	)
	tr := newTestTree(t, 600, 600, root, nil)
	tr.Frame()
	for key, want := range map[string]string{
		"sans":        "Inter",
		"mono":        "JetBrains Mono",
		"serif":       "Source Serif 4",
		"mostly-sans": "Inter",
		"mostly-mono": "JetBrains Mono",
		"arabic":      "Noto Sans Arabic",
		"empty-serif": "Source Serif 4",
		"block-mono":  "JetBrains Mono",
		"field":       "JetBrains Mono",
	} {
		n := mustFind(t, tr, key)
		if !strings.HasSuffix(n.Style.Font, " "+want) {
			t.Errorf("%s: font %q, want family %q", key, n.Style.Font, want)
		}
	}
}
