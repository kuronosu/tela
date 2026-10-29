package semantics

import (
	"strings"
	"testing"
)

func TestParseSnapshotLine(t *testing.T) {
	for _, tc := range []struct {
		line         string
		ok           bool
		role, name   string
		hasName, dis bool
		withRef      string
	}{
		{line: `- heading "Settings" [level=1]`, ok: true, role: "heading", name: "Settings", hasName: true,
			withRef: `- heading "Settings" [level=1] [ref=e1]`},
		{line: `  - button "Save" [disabled]`, ok: true, role: "button", name: "Save", hasName: true, dis: true,
			withRef: `  - button "Save" [disabled] [ref=e1]`},
		{line: `- textbox "Email": ana@example.com`, ok: true, role: "textbox", name: "Email", hasName: true,
			withRef: `- textbox "Email" [ref=e1]: ana@example.com`},
		{line: `- group "Theme":`, ok: true, role: "group", name: "Theme", hasName: true,
			withRef: `- group "Theme" [ref=e1]:`},
		{line: `- 'button "Count: 0"'`, ok: true, role: "button", name: "Count: 0", hasName: true,
			withRef: `- 'button "Count: 0" [ref=e1]'`},
		{line: `- 'button "it''s \"here\""': x`, ok: true, role: "button", name: `it's "here"`, hasName: true,
			withRef: `- 'button "it''s \"here\"" [ref=e1]': x`},
		{line: `- paragraph: Some text`, ok: true, role: "paragraph",
			withRef: `- paragraph [ref=e1]: Some text`},
		{line: `- switch "Wifi" [checked] [disabled]`, ok: true, role: "switch", name: "Wifi", hasName: true, dis: true,
			withRef: `- switch "Wifi" [checked] [disabled] [ref=e1]`},
		{line: `- button /a+b/ [pressed]`, ok: true, role: "button", name: "/a+b/", hasName: true,
			withRef: `- button /a+b/ [pressed] [ref=e1]`},
		{line: `- button / [pressed]`, ok: true, role: "button", name: "/", hasName: true,
			withRef: `- button / [pressed] [ref=e1]`},
		{line: `- text: Hello`},
		{line: `  - /url: https://example.com`},
		{line: `- "quoted value"`},
	} {
		l, ok := ParseSnapshotLine(tc.line)
		if ok != tc.ok {
			t.Errorf("%q: ok = %v, want %v", tc.line, ok, tc.ok)
			continue
		}
		if !ok {
			continue
		}
		if l.Role != tc.role || l.Name != tc.name || l.HasName != tc.hasName || l.Disabled != tc.dis {
			t.Errorf("%q: role %q name %q (has %v) disabled %v", tc.line, l.Role, l.Name, l.HasName, l.Disabled)
		}
		if got := l.WithSuffix(" [ref=e1]"); got != tc.withRef {
			t.Errorf("%q with a ref:\n got %s\nwant %s", tc.line, got, tc.withRef)
		}
	}
}

// TestSnapshotLineRoundTrip parses what Snapshot writes: every state the
// writer prints, every quoting case of a key and the names printed as
// regular expressions. The parser reads back the role, the name and the
// states, and a suffix keeps the key valid YAML that parses to the longer
// key.
func TestSnapshotLineRoundTrip(t *testing.T) {
	type el struct {
		n      Node
		role   string
		name   string // the name the line prints; "" for none
		states string
		dis    bool
		value  string // the text after the key
	}
	long := strings.Repeat("x", 901) // over 900 UTF-16 units: not printed
	cases := []el{
		{n: Node{Role: "button", Name: "Save"}, role: "button", name: "Save"},
		{n: Node{Role: "button"}, role: "button"},
		{n: Node{Role: "button", Name: long}, role: "button"},
		{n: Node{Role: "checkbox", Name: "Agree", Checked: ToggleTrue}, role: "checkbox", name: "Agree", states: " [checked]"},
		{n: Node{Role: "checkbox", Name: "Some", Checked: ToggleMixed}, role: "checkbox", name: "Some", states: " [checked=mixed]"},
		{n: Node{Role: "switch", Name: "Wifi", Checked: ToggleTrue, States: StateDisabled}, role: "switch", name: "Wifi", states: " [checked] [disabled]", dis: true},
		{n: Node{Role: "button", Name: "Menu", Expanded: ToggleTrue}, role: "button", name: "Menu", states: " [expanded]"},
		{n: Node{Role: "textbox", Name: "Email", Invalid: ToggleTrue, Value: "a@b.c"}, role: "textbox", name: "Email", states: " [invalid]", value: ": a@b.c"},
		{n: Node{Role: "heading", Name: "Title", Level: 2}, role: "heading", name: "Title", states: " [level=2]"},
		{n: Node{Role: "button", Name: "Bold", Pressed: ToggleTrue}, role: "button", name: "Bold", states: " [pressed]"},
		{n: Node{Role: "button", Name: "Bold", Pressed: ToggleMixed}, role: "button", name: "Bold", states: " [pressed=mixed]"},
		{n: Node{Role: "tab", Name: "One", Selected: ToggleTrue}, role: "tab", name: "One", states: " [selected]"},
		{n: Node{Role: "gridcell", Name: "A1", Selected: ToggleTrue, Expanded: ToggleTrue, Invalid: ToggleTrue, States: StateDisabled},
			role: "gridcell", name: "A1", states: " [disabled] [expanded] [invalid] [selected]", dis: true},
		// Names that make the key need quotes.
		{n: Node{Role: "button", Name: "Count: 0"}, role: "button", name: "Count: 0"},
		{n: Node{Role: "button", Name: "it's"}, role: "button", name: "it's"},
		{n: Node{Role: "button", Name: `say "hi"`}, role: "button", name: `say "hi"`},
		{n: Node{Role: "button", Name: `a\b`}, role: "button", name: `a\b`},
		{n: Node{Role: "button", Name: "a #b"}, role: "button", name: "a #b"},
		{n: Node{Role: "button", Name: "- item"}, role: "button", name: "- item"},
		{n: Node{Role: "button", Name: "end:"}, role: "button", name: "end:"},
		{n: Node{Role: "button", Name: "tab\x01ctl"}, role: "button", name: "tab\x01ctl"},
		{n: Node{Role: "button", Name: "emoji \U0001f600 ñ 日本"}, role: "button", name: "emoji \U0001f600 ñ 日本"},
		{n: Node{Role: "link", Name: "a: b", URL: "https://x.test/?a=b"}, role: "link", name: "a: b"},
		// Names printed as regular expressions, with and without states.
		{n: Node{Role: "button", Name: "/a+b/"}, role: "button", name: "/a+b/"},
		{n: Node{Role: "button", Name: "/a+b/", Pressed: ToggleTrue, States: StateDisabled}, role: "button", name: "/a+b/", states: " [disabled] [pressed]", dis: true},
		{n: Node{Role: "button", Name: "/"}, role: "button", name: "/"},
		{n: Node{Role: "button", Name: "/", Pressed: ToggleTrue}, role: "button", name: "/", states: " [pressed]"},
		{n: Node{Role: "button", Name: "//"}, role: "button", name: "//"},
		{n: Node{Role: "button", Name: "/a/b/"}, role: "button", name: "/a/b/"},
		{n: Node{Role: "button", Name: "/a [checked]/"}, role: "button", name: "/a [checked]/"},
		{n: Node{Role: "button", Name: "/a/ [disabled]/"}, role: "button", name: "/a/ [disabled]/"},
		{n: Node{Role: "button", Name: "/a: b/"}, role: "button", name: "/a: b/"},
		{n: Node{Role: "button", Name: "/it's/"}, role: "button", name: "/it's/"},
	}
	root := &Node{Kind: KindRoot}
	for i := range cases {
		n := cases[i].n
		n.Kind = KindElement
		root.Children = append(root.Children, &n)
	}
	lines := strings.Split(Snapshot(root), "\n")
	// A link has a /url line under it; element lines are the others.
	var got []string
	for _, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "- /url:") || strings.HasPrefix(strings.TrimSpace(line), "- text:") {
			continue
		}
		got = append(got, line)
	}
	if len(got) != len(cases) {
		t.Fatalf("Snapshot wrote %d element lines for %d elements:\n%s", len(got), len(cases), strings.Join(lines, "\n"))
	}
	for i, c := range cases {
		line := got[i]
		l, ok := ParseSnapshotLine(line)
		if !ok {
			t.Errorf("%q: not parsed", line)
			continue
		}
		if l.Role != c.role || l.Name != c.name || l.HasName != (c.name != "") || l.States != c.states || l.Disabled != c.dis {
			t.Errorf("%q: role %q name %q (has %v) states %q disabled %v; want %q %q %q %v", line, l.Role, l.Name, l.HasName, l.States, l.Disabled, c.role, c.name, c.states, c.dis)
		}
		wantRest := c.value
		if c.n.Role == "link" {
			wantRest = ":" // its /url is on the next line
		}
		if l.Rest != wantRest {
			t.Errorf("%q: rest %q, want %q", line, l.Rest, wantRest)
		}
		if l.Quoted != (yamlNeedsQuotes(l.Key)) {
			t.Errorf("%q: quoted = %v, but the key %q needs quotes: %v", line, l.Quoted, l.Key, yamlNeedsQuotes(l.Key))
		}
		// A suffix keeps the line parsing, with the longer key.
		const suffix = " [ref=e12]"
		again, ok := ParseSnapshotLine(l.WithSuffix(suffix))
		if !ok {
			t.Errorf("%q with a suffix: %q is not parsed", line, l.WithSuffix(suffix))
			continue
		}
		if again.Key != l.Key+suffix || again.Role != l.Role || again.Rest != l.Rest || again.Indent != l.Indent {
			t.Errorf("%q with a suffix: key %q rest %q, want %q and %q", line, again.Key, again.Rest, l.Key+suffix, l.Rest)
		}
		if again.Quoted != yamlNeedsQuotes(again.Key) {
			t.Errorf("%q with a suffix: quoted = %v, but the key %q needs quotes: %v", line, again.Quoted, again.Key, yamlNeedsQuotes(again.Key))
		}
	}
}

// TestSnapshotLineNested checks the indentation of a nested element.
func TestSnapshotLineNested(t *testing.T) {
	root := &Node{Kind: KindRoot, Children: []*Node{
		{Kind: KindElement, Role: "group", Name: "Theme", Children: []*Node{
			{Kind: KindElement, Role: "radio", Name: "Dark", Checked: ToggleTrue},
		}},
	}}
	lines := strings.Split(Snapshot(root), "\n")
	if len(lines) != 2 {
		t.Fatalf("lines %q", lines)
	}
	l, ok := ParseSnapshotLine(lines[1])
	if !ok || l.Indent != "  " || l.Role != "radio" || l.States != " [checked]" {
		t.Errorf("%q parsed as %+v, %v", lines[1], l, ok)
	}
	if got := l.WithSuffix(" [ref=e2]"); got != `  - radio "Dark" [checked] [ref=e2]` {
		t.Errorf("with a suffix: %s", got)
	}
	if d := l.Describe(); d != `radio "Dark"` {
		t.Errorf("Describe() = %s", d)
	}
}
