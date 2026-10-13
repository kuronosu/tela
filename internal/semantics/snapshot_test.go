package semantics

import (
	"encoding/json"
	"os"
	"slices"
	"strings"
	"testing"
)

func TestRolesSorted(t *testing.T) {
	if !slices.IsSorted(roles) {
		t.Fatal("roles must be sorted (ValidRole searches them)")
	}
	for _, r := range []string{"button", "switch", "treeitem", "alert", "none"} {
		if !ValidRole(r) {
			t.Errorf("ValidRole(%q) = false", r)
		}
	}
	for _, r := range []string{"Button", "widget", "command", "", "image"} {
		if ValidRole(r) {
			t.Errorf("ValidRole(%q) = true (abstract, misspelled or not ARIA 1.2)", r)
		}
	}
}

func TestParseAndSuggestRole(t *testing.T) {
	for _, tc := range []struct{ spec, role, bad string }{
		{"button", "button", ""},
		{"switch checkbox", "switch", ""},
		{"toggle switch", "switch", "toggle"},
		{"  ", "", ""},
		{"buton", "", "buton"},
	} {
		role, bad := ParseRole(tc.spec)
		if role != tc.role || bad != tc.bad {
			t.Errorf("ParseRole(%q) = %q, %q; want %q, %q", tc.spec, role, bad, tc.role, tc.bad)
		}
	}
	for in, want := range map[string]string{
		"buton": "button", "Switch": "switch", "chekbox": "checkbox", "heading1": "heading",
		"listitems": "listitem", "dialogue": "dialog", "xyzzyplugh": "",
	} {
		if got := SuggestRole(in); got != want {
			t.Errorf("SuggestRole(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNormalizeWhiteSpace(t *testing.T) {
	for in, want := range map[string]string{
		"":                       "",
		"  a  b\t\nc ":           "a b c",
		"a\u00a0\u2003b":         "a b",
		"zero\u200bwidth\u00ad.": "zerowidth.",
		"\ufeffbom":              "bom",
		"x\u0085y":               "x\u0085y", // NEL is not JavaScript white space
	} {
		if got := NormalizeWhiteSpace(in); got != want {
			t.Errorf("NormalizeWhiteSpace(%q) = %q, want %q", in, got, want)
		}
	}
}

// TestYAMLQuoting checks Playwright's quoting rules. The values were
// compared with Playwright 1.61's yamlEscapeValueIfNeeded and
// yamlEscapeKeyIfNeeded run by Node (TELA_YAML_CASES=file.json writes the
// inputs with their results for such a check).
func TestYAMLQuoting(t *testing.T) {
	values := map[string]string{
		"plain":        "plain",
		"":             `""`,
		" lead":        `" lead"`,
		"trail ":       `"trail "`,
		"- item":       `"- item"`,
		"a: b":         `"a: b"`,
		"a:b":          "a:b",
		"end:":         `"end:"`,
		"x #c":         `"x #c"`,
		"x#c":          "x#c",
		"#c":           `"#c"`,
		"line\nbreak":  `"line\nbreak"`,
		"cr\r":         `"cr\r"`,
		"tab\there":    "tab\there",
		"bell\a":       `"bell\x07"`,
		"del\u007f":    `"del\x7f"`,
		"c1\u0085":     `"c1\x85"`,
		"{brace":       `"{brace"`,
		"tick`":        "\"tick`\"",
		"[list":        `"[list"`,
		"&anchor":      `"&anchor"`,
		"*alias":       `"*alias"`,
		"!tag":         `"!tag"`,
		"|block":       `"|block"`,
		">fold":        `">fold"`,
		"@at":          `"@at"`,
		"%pct":         `"%pct"`,
		",comma":       `",comma"`,
		"?q":           `"?q"`,
		"]b":           `"]b"`,
		`"q"`:          `"\"q\""`,
		"'s'":          `"'s'"`,
		`back\slash`:   `back\slash`,
		"say \"hi\"":   `say "hi"`,
		"42":           `"42"`,
		"-1.5":         `"-1.5"`,
		"+3":           `"+3"`,
		".5":           `".5"`,
		"5.":           `"5."`,
		"1e10":         `"1e10"`,
		"1e":           "1e",
		"0x1f":         `"0x1f"`,
		"0b101":        `"0b101"`,
		"0o17":         `"0o17"`,
		"0x":           "0x",
		"Infinity":     `"Infinity"`,
		"NaN":          "NaN",
		"1_000":        "1_000",
		"Yes":          `"Yes"`,
		"null":         `"null"`,
		"OFF":          `"OFF"`,
		"nope":         "nope",
		"ünïcödé ✓ 日本": "ünïcödé ✓ 日本",
	}
	keys := map[string]string{
		`button "Save"`:                `button "Save"`,
		`heading "Count: 0" [level=1]`: `'heading "Count: 0" [level=1]'`,
		`button "it's: ok"`:            `'button "it''s: ok"'`,
	}
	if path := os.Getenv("TELA_YAML_CASES"); path != "" {
		js, _ := json.Marshal(map[string]map[string]string{"values": values, "keys": keys})
		if err := os.WriteFile(path, js, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for in, want := range values {
		if got := yamlEscapeValue(in); got != want {
			t.Errorf("yamlEscapeValue(%q) = %s, want %s", in, got, want)
		}
	}
	for in, want := range keys {
		if got := yamlEscapeKey(in); got != want {
			t.Errorf("yamlEscapeKey(%q) = %s, want %s", in, got, want)
		}
	}
}

func TestJSONString(t *testing.T) {
	for in, want := range map[string]string{
		"a":          `"a"`,
		`q"b\`:       `"q\"b\\"`,
		"\b\f\n\r\t": `"\b\f\n\r\t"`,
		"\x01\x1f":   `"\u0001\u001f"`,
		"\x7f<>&é":   "\"\x7f<>&é\"",
		"\u2028":     "\"\u2028\"",
		"bad\xff":    "\"bad\uFFFD\"",
	} {
		if got := jsonString(in); got != want {
			t.Errorf("jsonString(%q) = %s, want %s", in, got, want)
		}
	}
}

// el returns an element node.
func el(role string, explicit bool, name string, children ...*Node) *Node {
	return &Node{Kind: KindElement, Role: role, ExplicitRole: explicit, Name: name, Children: children}
}

func txt(s string) *Node { return &Node{Kind: KindText, Text: s, Inline: true} }

func inline(n *Node) *Node { n.Inline = true; return n }

func TestSnapshotTrees(t *testing.T) {
	hidden := el("button", false, "Hidden", txt("Hidden"))
	hidden.States = StateHidden
	disabledList := el("listitem", true, "")
	disabledList.States = StateDisabled // listitem has no disabled state
	disabledList.Level = 2
	disabledList.Children = []*Node{txt("Item")}
	box := el("textbox", true, "Name")
	box.Value = "  Ada \n Lovelace "
	box.Children = []*Node{txt("ignored")}
	link := inline(el("link", false, "Docs", txt("Docs")))
	link.URL = "/docs"
	linkNoText := el("link", false, "")
	linkNoText.URL = "#top"
	for _, tc := range []struct {
		name string
		root *Node
		want string
	}{
		{"empty", &Node{Kind: KindRoot}, ""},
		{"nil", nil, ""},
		{"generic containers flatten", &Node{Kind: KindRoot, Children: []*Node{
			el("generic", false, "", el("generic", false, "", txt("a")), txt("b")),
			inline(el("generic", false, "", txt("c"))), txt("d"),
		}}, "- text: a b cd"},
		{"empty explicit generic is dropped", &Node{Kind: KindRoot, Children: []*Node{
			el("generic", true, ""), el("generic", true, "Named"), el("generic", true, "", txt("x")),
		}}, "- generic \"Named\"\n- generic: x"},
		{"hidden nodes and their subtrees are left out", &Node{Kind: KindRoot, Children: []*Node{
			hidden, el("button", false, "Shown", txt("Shown")),
		}}, "- button \"Shown\""},
		{"states only where the role has them", &Node{Kind: KindRoot, Children: []*Node{disabledList}},
			"- listitem [level=2]: Item"},
		{"textbox value", &Node{Kind: KindRoot, Children: []*Node{box}}, "- textbox \"Name\": Ada Lovelace"},
		{"links", &Node{Kind: KindRoot, Children: []*Node{el("paragraph", false, "", txt("See "), link), linkNoText}},
			"- paragraph:\n  - text: See\n  - link \"Docs\":\n    - /url: /docs\n- link:\n  - /url: \"#top\""},
		{"a non-root node renders itself", el("button", false, "Go", txt("Go")), "- button \"Go\""},
		{"none and presentation flatten", &Node{Kind: KindRoot, Children: []*Node{
			el("none", true, "", el("button", false, "A")), el("presentation", true, "", txt("t")),
		}}, "- button \"A\"\n- text: t"},
	} {
		if got := Snapshot(tc.root); got != tc.want {
			t.Errorf("%s:\n%s\nwant:\n%s", tc.name, got, tc.want)
		}
	}
}

func TestToggleStates(t *testing.T) {
	sw := el("switch", true, "S")
	sw.Checked = ToggleMixed
	b := el("button", false, "B")
	b.Pressed, b.Expanded = ToggleMixed, ToggleTrue
	tab := el("tab", true, "T")
	tab.Selected, tab.Expanded, tab.Checked = ToggleTrue, ToggleFalse, ToggleTrue
	got := Snapshot(&Node{Kind: KindRoot, Children: []*Node{sw, b, tab}})
	want := strings.Join([]string{
		`- switch "S" [checked=mixed]`,
		`- button "B" [expanded] [pressed=mixed]`,
		`- tab "T" [selected]`,
	}, "\n")
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}
