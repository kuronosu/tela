package core

import (
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/semantics"
	"github.com/kuronosu/tela/internal/style"
)

func h2(c ...Child) Node     { return Primitive(style.KindH2, c...) }
func anchor(c ...Child) Node { return Primitive(style.KindA, c...) }

// ariaCase is a tree, the aria snapshot Tela gives for it and the HTML
// whose Playwright snapshot it matches.
type ariaCase struct {
	name string
	root func() Node
	want string
	// html is the equivalent page body. The wants were checked byte for
	// byte against Playwright 1.61's locator("body").ariaSnapshot() in
	// Chromium on this HTML (TELA_ARIA_CASES=file.json go test -run
	// TestAriaSnapshotCases writes the cases for such a check).
	html string
}

var ariaCases = []ariaCase{
	{
		name: "counter",
		root: func() Node {
			return div(Class("flex flex-col items-center gap-6 p-10"),
				h1(Class("text-5xl font-bold"), Text("Count: 0")),
				button(Class("px-6 py-3"), On("click", func(MouseEvent) {}), Text("Increment")))
		},
		want: "- 'heading \"Count: 0\" [level=1]'\n- button \"Increment\"",
		html: `<div style="display:flex;flex-direction:column"><h1>Count: 0</h1><button>Increment</button></div>`,
	},
	{
		name: "architecture section 16",
		root: func() Node {
			return div(
				h1(Text("Settings")),
				// Tela has no text input yet: a textbox shows its content
				// as its value, like an input's value.
				div(Role("textbox"), AriaLabel("Email"), Text("ana@example.com")),
				div(Role("checkbox"), AriaChecked(true), Text("Notifications")),
				button(Text("Save")))
		},
		want: "- heading \"Settings\" [level=1]\n- textbox \"Email\": ana@example.com\n- checkbox \"Notifications\" [checked]\n- button \"Save\"",
		html: `<div><h1>Settings</h1><input aria-label="Email" value="ana@example.com"><div role="checkbox" aria-checked="true">Notifications</div><button>Save</button></div>`,
	},
	{
		name: "inline elements",
		root: func() Node {
			return p(Text("Hello "), strong(Text("world")), Text(", "), em(Text("again")), Text("!"))
		},
		want: "- paragraph:\n  - text: Hello\n  - strong: world\n  - text: \",\"\n  - emphasis: again\n  - text: \"!\"",
		html: `<p>Hello <strong>world</strong>, <em>again</em>!</p>`,
	},
	{
		name: "texts merge across containers",
		root: func() Node {
			return div(div(Text("Alpha")), div(span(Text("Beta")), Text(" gamma")), span(Text("del"), Text("ta")))
		},
		want: "- text: Alpha Beta gamma delta",
		html: `<div><div>Alpha</div><div><span>Beta</span> gamma</div><span>del</span>ta</div>`,
	},
	{
		name: "flex items are blocks",
		root: func() Node {
			return div(Class("flex"), span(Text("One")), span(Text("Two")))
		},
		want: "- text: One Two",
		html: `<div style="display:flex"><span>One</span><span>Two</span></div>`,
	},
	{
		name: "links",
		root: func() Node {
			return div(
				p(Text("Read the "), anchor(Href("/docs"), Text("docs")), Text(".")),
				anchor(Text("no href")),
				anchor(Href("https://example.com/a?b=c"), AriaLabel("Example")))
		},
		want: "- paragraph:\n  - text: Read the\n  - link \"docs\":\n    - /url: /docs\n  - text: .\n- text: no href\n- link \"Example\":\n  - /url: https://example.com/a?b=c",
		html: `<div><p>Read the <a href="/docs">docs</a>.</p><a>no href</a><a href="https://example.com/a?b=c" aria-label="Example"></a></div>`,
	},
	{
		name: "images",
		root: func() Node {
			return div(
				img(Alt("Logo")),
				img(Alt("")),
				img(),
				img(Alt(""), AriaLabel("Chart")),
				button(img(Alt("Close"))))
		},
		want: "- img \"Logo\"\n- img\n- img \"Chart\"\n- button \"Close\":\n  - img \"Close\"",
		html: `<div><img alt="Logo"><img alt=""><img><img alt="" aria-label="Chart"><button><img alt="Close"></button></div>`,
	},
	{
		name: "hidden subtrees",
		root: func() Node {
			return div(
				div(AriaHidden(true), button(Text("Hidden by aria"))),
				div(Class("hidden"), button(Text("Hidden by display"))),
				div(Class("invisible"), button(Text("Hidden by visibility"))),
				button(Text("Shown "), span(AriaHidden(true), Text("icon")), span(Class("hidden"), Text("gone"))))
		},
		want: "- button \"Shown\"",
		html: `<div><div aria-hidden="true"><button>Hidden by aria</button></div><div style="display:none"><button>Hidden by display</button></div><div style="visibility:hidden"><button>Hidden by visibility</button></div><button>Shown <span aria-hidden="true">icon</span><span style="display:none">gone</span></button></div>`,
	},
	{
		name: "names from content",
		root: func() Node {
			return div(
				button(span(Text("Save")), span(Text("draft"))),
				button(Class("flex"), span(Text("Open")), span(Text("file"))),
				button(Text("Sign "), strong(Text("in"))),
				h2(Text("  Spaced \n  out  ")),
				button(AriaLabel("Settings"), span(Text("⚙"))),
				button(p(Text("Para")), div(Role("heading"), Text("Head"))),
				button(div(Role("navigation"), Text("Nav")), Text("End")),
			)
		},
		want: "- button \"Savedraft\"\n- button \"Open file\"\n- button \"Sign in\":\n  - text: Sign\n  - strong: in\n- heading \"Spaced out\" [level=2]\n- button \"Settings\": ⚙\n- button \"Para Head\":\n  - paragraph: Para\n  - heading \"Head\"\n- button \"End\":\n  - navigation: Nav\n  - text: End",
		html: `<div><button><span>Save</span><span>draft</span></button><button style="display:flex"><span>Open</span><span>file</span></button><button>Sign <strong>in</strong></button><h2>  Spaced
  out  </h2><button aria-label="Settings"><span>⚙</span></button><button><p>Para</p><div role="heading">Head</div></button><button><div role="navigation">Nav</div>End</button></div>`,
	},
	{
		name: "states",
		root: func() Node {
			return div(
				button(Disabled(true), Text("Go")),
				div(Disabled(true), button(Text("Inside"))),
				button(AriaDisabled(true), Text("Soft")),
				div(AriaDisabled(true), div(Role("switch"), AriaChecked(false), AriaLabel("Wi-Fi"))),
				div(Role("switch"), AriaChecked(true), AriaLabel("Bluetooth")),
				button(AriaExpanded(true), Text("Menu")),
				button(AriaExpanded(false), Text("More")),
				button(AriaPressed(true), Text("Bold")),
				div(Role("tab"), AriaSelected(true), Text("General")),
				div(Role("heading"), AriaLevel(3), Text("Sub")),
				div(Role("radio"), AriaChecked(true), Text("Small")),
				div(Role("checkbox"), Text("Unset")),
			)
		},
		want: "- button \"Go\" [disabled]\n- button \"Inside\" [disabled]\n- button \"Soft\" [disabled]\n- switch \"Wi-Fi\" [disabled]\n- switch \"Bluetooth\" [checked]\n- button \"Menu\" [expanded]\n- button \"More\"\n- button \"Bold\" [pressed]\n- tab \"General\" [selected]\n- heading \"Sub\" [level=3]\n- radio \"Small\" [checked]\n- checkbox \"Unset\"",
		html: `<div><button disabled>Go</button><fieldset role="none" disabled style="border:0;margin:0;padding:0"><button>Inside</button></fieldset><button aria-disabled="true">Soft</button><div aria-disabled="true"><div role="switch" aria-checked="false" aria-label="Wi-Fi"></div></div><div role="switch" aria-checked="true" aria-label="Bluetooth"></div><button aria-expanded="true">Menu</button><button aria-expanded="false">More</button><button aria-pressed="true">Bold</button><div role="tab" aria-selected="true">General</div><div role="heading" aria-level="3">Sub</div><div role="radio" aria-checked="true">Small</div><div role="checkbox">Unset</div></div>`,
	},
	{
		name: "roles and lists",
		root: func() Node {
			return div(
				div(Role("list"), div(Role("listitem"), Text("One")), div(Role("listitem"), Text("Two"))),
				div(Role("generic"), Text("explicit")),
				div(Role("generic")),
				div(Role("none"), Text("none")),
				button(Role("presentation"), Text("kept")),
				div(Role("navigation"), AriaLabel("Main"), anchor(Href("#a"), Text("A"))),
				div(Role("switch checkbox"), AriaChecked(true), Text("Fallback")),
				div(Role("group"), div(Role("button"), TabIndex(0), Text("Div button"))),
			)
		},
		want: "- list:\n  - listitem: One\n  - listitem: Two\n- generic: explicit\n- text: none\n- button \"kept\"\n- navigation \"Main\":\n  - link \"A\":\n    - /url: \"#a\"\n- switch \"Fallback\" [checked]\n- group:\n  - button \"Div button\"",
		html: `<div><div role="list"><div role="listitem">One</div><div role="listitem">Two</div></div><div role="generic">explicit</div><div role="generic"></div><div role="none">none</div><button role="presentation">kept</button><div role="navigation" aria-label="Main"><a href="#a">A</a></div><div role="switch checkbox" aria-checked="true">Fallback</div><div role="group"><div role="button" tabindex="0">Div button</div></div></div>`,
	},
	{
		name: "yaml quoting",
		root: func() Node {
			return div(
				div(Text("- dash")),
				p(Text("#tag")),
				p(Text("123")),
				p(Text("yes")),
				p(Text("a: b")),
				p(Text("it's {x}")),
				p(Text(`say "hi" \ bye`)),
				p(Text("0x1F")),
				p(Text("1e5")),
				p(Text("x #y")),
				button(Text("it's: ok")),
				button(Text(`quote "q"`)),
				button(Text("/path/")),
				h1(Text("[tag]")),
				p(Text("ends with:")),
				p(Text("@home")),
			)
		},
		want: "- text: \"- dash\"\n- paragraph: \"#tag\"\n- paragraph: \"123\"\n- paragraph: \"yes\"\n- paragraph: \"a: b\"\n- paragraph: \"it's {x}\"\n" +
			"- paragraph: say \"hi\" \\ bye\n- paragraph: \"0x1F\"\n- paragraph: \"1e5\"\n- paragraph: \"x #y\"\n- 'button \"it''s: ok\"'\n" +
			"- button \"quote \\\"q\\\"\"\n- button /path/\n- heading \"[tag]\" [level=1]\n- paragraph: \"ends with:\"\n- paragraph: \"@home\"",
		html: `<div><div>- dash</div><p>#tag</p><p>123</p><p>yes</p><p>a: b</p><p>it's {x}</p><p>say "hi" \ bye</p><p>0x1F</p><p>1e5</p><p>x #y</p><button>it's: ok</button><button>quote "q"</button><button>/path/</button><h1>[tag]</h1><p>ends with:</p><p>@home</p></div>`,
	},
	{
		name: "settings row and radio group",
		root: func() Node { return Component(settingsFragment) },
		want: "- text: \"Security alerts Always on: sign-ins from new devices.\"\n- switch \"Security alerts\" [checked] [disabled]\n" +
			"- text: Theme\n- radiogroup \"Theme\":\n  - radio \"Light\"\n  - radio \"DarkNight mode\" [checked]",
		html: `<div style="display:flex"><div style="display:flex;flex-direction:column"><span>Security alerts</span><span>Always on: sign-ins from new devices.</span></div><button role="switch" aria-checked="true" aria-label="Security alerts" disabled><div></div></button></div>` +
			`<div style="display:flex;flex-direction:column"><span id="theme">Theme</span><div role="radiogroup" aria-labelledby="theme" style="display:flex"><button role="radio" aria-checked="false"><div></div><div style="display:flex">Light</div></button><button role="radio" aria-checked="true"><div><span>Dark</span><span>Night mode</span></div></button></div></div>`,
	},
	{
		name: "white space and values",
		root: func() Node {
			return div(
				p(Text("tab\there\u00a0nbsp\u200bzero")),
				button(Text("Volume "), div(Role("slider"), AriaValueNow(5), AriaValueText("5 of 10"))),
				button(Text("Level "), div(Role("progressbar"), AriaValueNow(0.25))),
				h1(Text(strings.Repeat("long ", 190))),
				div(Class("invisible"), button(Class("visible"), Text("Peek"))),
				button(Text("x"), span(Class("invisible"), Text("y"))),
			)
		},
		want: "- paragraph: tab here nbspzero\n- button \"Volume 5 of 10\":\n  - text: Volume\n  - slider\n- button \"Level 0.25\":\n  - text: Level\n  - progressbar\n" +
			"- heading [level=1]\n- button \"x\"", // names over 900 characters are not printed
		html: `<div><p>tab	here&nbsp;nbsp&#8203;zero</p><button>Volume <div role="slider" aria-valuenow="5" aria-valuetext="5 of 10"></div></button><button>Level <div role="progressbar" aria-valuenow="0.25"></div></button><h1>` +
			strings.Repeat("long ", 190) + `</h1><div style="visibility:hidden"><button style="visibility:visible">Peek</button></div><button>x<span style="visibility:hidden">y</span></button></div>`,
	},
	{
		name: "labelled by",
		root: func() Node { return Component(labelledByScreen) },
		want: "- dialog \"Delete file?\":\n  - heading \"Delete file?\" [level=2]\n  - button \"Delete\"\n- region \"Hidden title\": Body\n- button \"Own label\": text",
		html: `<div><div role="dialog" aria-labelledby="t"><h2 id="t">Delete file?</h2><button>Delete</button></div><div role="region" aria-labelledby="h"><span id="h" style="display:none">Hidden title</span>Body</div><button aria-labelledby="b" aria-label="ignored"><span id="b" aria-label="Own label">text</span></button></div>`,
	},
}

func settingsFragment() Node {
	theme := UseElementRef()
	return Fragment(
		div(Class("flex"),
			div(Class("flex flex-col"), span(Text("Security alerts")), span(Text("Always on: sign-ins from new devices."))),
			button(Role("switch"), AriaChecked(true), AriaLabel("Security alerts"), Disabled(true), div())),
		div(Class("flex flex-col"),
			span(AttachRef(theme), Text("Theme")),
			div(Class("flex"), Role("radiogroup"), AriaLabelledBy(theme),
				button(Role("radio"), AriaChecked(false), div(), div(Class("flex"), Text("Light"))),
				button(Role("radio"), AriaChecked(true), div(span(Text("Dark")), span(Text("Night mode")))))),
	)
}

func labelledByScreen() Node {
	title, hidden, own := UseElementRef(), UseElementRef(), UseElementRef()
	return div(
		div(Role("dialog"), AriaLabelledBy(title), h2(AttachRef(title), Text("Delete file?")), button(Text("Delete"))),
		div(Role("region"), AriaLabelledBy(hidden), span(AttachRef(hidden), Class("hidden"), Text("Hidden title")), Text("Body")),
		button(AriaLabelledBy(own), AriaLabel("ignored"), span(AttachRef(own), AriaLabel("Own label"), Text("text"))),
	)
}

// snapshotOf mounts root in an 800x600 tree, runs a frame and returns its
// aria snapshot.
func snapshotOf(t testing.TB, root Node) (*Tree, string) {
	t.Helper()
	tr := newTestTree(t, 800, 600, root, nil)
	tr.Frame()
	return tr, tr.AriaSnapshot()
}

func TestAriaSnapshotCases(t *testing.T) {
	if path := os.Getenv("TELA_ARIA_CASES"); path != "" {
		var out []map[string]string
		for _, c := range ariaCases {
			out = append(out, map[string]string{"name": c.name, "html": c.html, "want": c.want})
		}
		js, _ := json.MarshalIndent(out, "", "  ")
		if err := os.WriteFile(path, js, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range ariaCases {
		t.Run(c.name, func(t *testing.T) {
			_, got := snapshotOf(t, Component(c.root))
			if got != c.want {
				t.Errorf("aria snapshot:\n%s\nwant:\n%s", got, c.want)
			}
		})
	}
}

// findNode returns the first node of the tree with role and name.
func findNode(root *semantics.Node, role, name string) *semantics.Node {
	var found *semantics.Node
	root.Walk(func(n *semantics.Node) bool {
		if found == nil && n.Kind == semantics.KindElement && n.Role == role && n.Name == name {
			found = n
		}
		return found == nil
	})
	return found
}

func mustNode(t testing.TB, tr *Tree, role, name string) *semantics.Node {
	t.Helper()
	n := findNode(tr.Semantics(), role, name)
	if n == nil {
		t.Fatalf("no %s %q in\n%s", role, name, tr.AriaSnapshot())
	}
	return n
}

func TestSemanticsRolesAndStates(t *testing.T) {
	tr, _ := snapshotOf(t, div(
		div(Key("tab"), TabIndex(0), Text("Focus me")),
		div(Key("scroller"), Class("h-20 overflow-auto"), div(Class("h-96"))),
		button(Key("b"), Text("Go")),
		div(Role("slider"), AriaValueNow(30), AriaValueMin(0), AriaValueMax(100), AriaLabel("Volume")),
		div(Role("progressbar"), AriaValueNow(1), AriaValueText("Step 1 of 3"), AriaDescription("Upload")),
	))
	root := tr.Semantics()
	if root.Kind != semantics.KindRoot || root.ID == 0 {
		t.Fatalf("root %+v", root)
	}
	var focusable *semantics.Node
	root.Walk(func(n *semantics.Node) bool {
		if n.Role == "generic" && n.Has(semantics.StateFocusable) {
			focusable = n
		}
		return true
	})
	if focusable == nil || !focusable.Has(semantics.StateFocusable) || !focusable.Can(semantics.ActionFocus) || focusable.ExplicitRole {
		t.Errorf("a TabIndex div is a focusable generic node: %+v", focusable)
	}
	var scroller *semantics.Node
	root.Walk(func(n *semantics.Node) bool {
		if n.Has(semantics.StateScrollable) {
			scroller = n
		}
		return true
	})
	if scroller == nil || !scroller.Can(semantics.ActionScroll) || scroller.Bounds.Height() != 80 {
		t.Errorf("scroll container: %+v", scroller)
	}
	b := mustNode(t, tr, "button", "Go")
	if !b.Can(semantics.ActionClick|semantics.ActionFocus|semantics.ActionScrollIntoView) || b.Has(semantics.StateFocused) {
		t.Errorf("button actions %b states %b", b.Actions, b.States)
	}
	s := mustNode(t, tr, "slider", "Volume")
	if s.Value != "30" || !s.Range.HasNow || s.Range.Max != 100 || !s.Range.HasMin {
		t.Errorf("slider value %q range %+v", s.Value, s.Range)
	}
	pb := mustNode(t, tr, "progressbar", "")
	if pb.Value != "Step 1 of 3" || pb.Description != "Upload" {
		t.Errorf("progressbar value %q description %q", pb.Value, pb.Description)
	}

	// Focus shows as a state.
	if err := tr.FocusNode(b.ID); err != nil {
		t.Fatal(err)
	}
	tr.Frame()
	if b2 := mustNode(t, tr, "button", "Go"); !b2.Has(semantics.StateFocused) || b2.ID != b.ID {
		t.Errorf("after FocusNode: states %b, ID %d (was %d)", b2.States, b2.ID, b.ID)
	}
}

func TestSemanticsDisabledFocus(t *testing.T) {
	tr, _ := snapshotOf(t, div(Disabled(true), button(Text("Off"))))
	n := mustNode(t, tr, "button", "Off")
	if !n.Has(semantics.StateDisabled) || n.Has(semantics.StateFocusable) || n.Can(semantics.ActionClick) {
		t.Errorf("disabled button: states %b actions %b", n.States, n.Actions)
	}
	if err := tr.ClickNode(n.ID); !errors.Is(err, ErrNodeDisabled) {
		t.Errorf("ClickNode on a disabled button: %v", err)
	}
	if err := tr.FocusNode(n.ID); !errors.Is(err, ErrNotFocusable) {
		t.Errorf("FocusNode on a disabled button: %v", err)
	}
	if err := tr.ClickNode(123456); !errors.Is(err, ErrNodeGone) {
		t.Errorf("ClickNode on an unknown node: %v", err)
	}
}

func TestSemanticsHiddenNodesKept(t *testing.T) {
	tr, _ := snapshotOf(t, div(div(Class("hidden"), button(Text("Ghost")))))
	n := mustNode(t, tr, "button", "Ghost")
	if !n.Has(semantics.StateHidden) || !n.Bounds.Empty() || n.Can(semantics.ActionScrollIntoView) {
		t.Errorf("a button under display: none: states %b bounds %v actions %b", n.States, n.Bounds, n.Actions)
	}
}

func TestSemanticsCache(t *testing.T) {
	tr, _ := snapshotOf(t, div(button(Text("A"))))
	a, b := tr.Semantics(), tr.Semantics()
	if a != b {
		t.Error("an unchanged tree builds its semantics again")
	}
	tr.Mount(div(button(Text("B"))))
	tr.Frame()
	if c := tr.Semantics(); c == a || findNode(c, "button", "B") == nil {
		t.Error("the semantics did not follow a new frame")
	}
}

// semCounter is a counter whose button increments through OnClick.
func semCounter() Node {
	count, set := UseState(0)
	return div(Class("flex flex-col gap-4 p-8"),
		h1(Textf("Count: %d", count)),
		button(On("click", func(MouseEvent) { set.Set(count + 1) }), Text("Increment")))
}

func TestClickNodeIncrementsCounter(t *testing.T) {
	x := newInput(t, Component(semCounter), 400, 300)
	tr := x.tree()
	inc := mustNode(t, tr, "button", "Increment")
	for range 2 {
		if err := tr.ClickNode(inc.ID); err != nil {
			t.Fatal(err)
		}
	}
	x.frame()
	if got := tr.AriaSnapshot(); !strings.Contains(got, `heading "Count: 2"`) {
		t.Errorf("two semantic clicks:\n%s", got)
	}
	// The text of the button clicks the button.
	var label *semantics.Node
	mustNode(t, tr, "button", "Increment").Walk(func(n *semantics.Node) bool {
		if n.Kind == semantics.KindText {
			label = n
		}
		return true
	})
	if label == nil || label.Bounds.Empty() {
		t.Fatalf("button label %+v", label)
	}
	if err := tr.ClickNode(label.ID); err != nil {
		t.Fatal(err)
	}
	x.frame()
	if got := tr.AriaSnapshot(); !strings.Contains(got, `heading "Count: 3"`) {
		t.Errorf("a click on the button's text:\n%s", got)
	}
}

func TestClickNodeEvent(t *testing.T) {
	var got []MouseEvent
	tr, _ := snapshotOf(t, div(Class("p-10"), On("click", func(e MouseEvent) { got = append(got, e) }),
		button(Class("ml-20 h-10 w-40"), Text("Hit"))))
	n := mustNode(t, tr, "button", "Hit")
	if err := tr.ClickNode(n.ID); err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("%d clicks bubbled to the parent, want 1", len(got))
	}
	e := got[0]
	if e.X != 40+80+80 || e.Y != 40+20 || e.Button != MouseButtonLeft || e.ClickCount != 1 {
		t.Errorf("click event %+v; want the button's center (200, 60), primary button, count 1", e)
	}
}

func TestScrollNodeIntoView(t *testing.T) {
	rows := make([]Node, 30)
	for i := range rows {
		rows[i] = div(Class("h-10"), Textf("Row %d", i))
	}
	rows[25] = button(Class("h-10"), Text("Target"))
	x := newInput(t, div(Key("list"), Class("h-40 overflow-y-auto"), Fragment(childrenOf(rows)...)), 400, 300)
	tr := x.tree()
	n := mustNode(t, tr, "button", "Target")
	if n.Bounds.Y0 != 1000 {
		t.Fatalf("target at %v before scrolling", n.Bounds)
	}
	if err := tr.ScrollNodeIntoView(n.ID); err != nil {
		t.Fatal(err)
	}
	x.frame()
	n = mustNode(t, tr, "button", "Target")
	// The list is 160 px high: the target's bottom edge meets it.
	if n.Bounds.Y1 != 160 || n.Bounds.Y0 != 120 {
		t.Errorf("target at %v after ScrollNodeIntoView, want y 120 to 160", n.Bounds)
	}
	if list := keyed(tr, "list"); list.box.scr.pos.Y != 880 {
		t.Errorf("list scrolled to %v, want 880", list.box.scr.pos.Y)
	}
}

func childrenOf(nodes []Node) []Child {
	out := make([]Child, len(nodes))
	for i, n := range nodes {
		out[i] = n
	}
	return out
}

func TestSemanticsBounds(t *testing.T) {
	tr, _ := snapshotOf(t, div(Class("p-10"),
		button(Class("block h-10 w-20"), Text("Plain")),
		div(Class("ml-10 translate-x-5 scale-200"),
			button(Class("block h-10 w-20"), Text("Moved"))),
		div(Class("h-20 overflow-y-auto"), Key("sc"),
			div(Class("h-40")), button(Class("block h-10 w-20"), Text("Scrolled"))),
		p(Text("Go to "), anchor(Href("/x"), Text("docs"))),
	))
	if b := mustNode(t, tr, "button", "Plain").Bounds; b != (gfx.Rect{X0: 40, Y0: 40, X1: 120, Y1: 80}) {
		t.Errorf("plain button %v", b)
	}
	// The parent spans x 80..760 (ml-10 inside p-10) and is 40 high at y
	// 80; scale-200 around its center (420, 100) puts the button's x
	// 80..160 at -260..-100, and translate-x-5 moves it 20 px.
	if b := mustNode(t, tr, "button", "Moved").Bounds; b != (gfx.Rect{X0: -260 + 20, Y0: 60, X1: -100 + 20, Y1: 140}) {
		t.Errorf("transformed button %v", b)
	}
	sc := keyed(tr, "sc")
	sc.box.scr.pos.Y = 100 // painted offsets follow at the next paint
	tr.requestRepaint()
	tr.Frame()
	if b := mustNode(t, tr, "button", "Scrolled").Bounds; b != (gfx.Rect{X0: 40, Y0: 120 + 160 - 100, X1: 120, Y1: 120 + 200 - 100}) {
		t.Errorf("scrolled button %v", b)
	}
	link := mustNode(t, tr, "link", "docs")
	para := mustNode(t, tr, "paragraph", "")
	if link.Bounds.Empty() || link.Bounds.X0 <= para.Bounds.X0 || !para.Bounds.ContainsRect(link.Bounds) {
		t.Errorf("inline link %v in paragraph %v", link.Bounds, para.Bounds)
	}
}

func TestRolePanicsInDevMode(t *testing.T) {
	devTree(t, div(), nil) // dev mode on
	msg := panicMessage(func() { Role("buton") })
	if !strings.Contains(msg, `unknown ARIA role "buton"`) || !strings.Contains(msg, `did you mean "button"?`) {
		t.Errorf("Role(\"buton\") in dev mode: %q", msg)
	}
	if msg := panicMessage(func() { Role("Switch") }); !strings.Contains(msg, `did you mean "switch"?`) {
		t.Errorf("Role(\"Switch\"): %q", msg)
	}
	if msg := panicMessage(func() { Role("switch checkbox") }); msg != "" {
		t.Errorf("a valid fallback list panics: %q", msg)
	}
	if msg := panicMessage(func() { img(Alt("x")); div(Alt("x")) }); !strings.Contains(msg, "only Img") {
		t.Errorf("Alt on a div: %q", msg)
	}
}

// BenchmarkSemantics10k builds the semantics tree of about 10,000
// Elements (8,568 nodes: rows, spans, texts, divs) from scratch, as
// Semantics does after every frame that changed something.
func BenchmarkSemantics10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
	tr.Frame()
	nodes := 0
	tr.Semantics().Walk(func(*semantics.Node) bool { nodes++; return true })
	b.ReportAllocs()
	for b.Loop() {
		tr.sem.valid = false
		tr.Semantics()
	}
	b.ReportMetric(float64(nodes), "nodes")
}

// BenchmarkAriaSnapshot10k serializes the semantics tree of
// BenchmarkSemantics10k (built once) as an aria snapshot.
func BenchmarkAriaSnapshot10k(b *testing.B) {
	setters := make([]func(string), benchRows)
	tr := newTestTree(b, 800, 600, bigList(benchRows, setters), nil)
	tr.Frame()
	root := tr.Semantics()
	b.ReportAllocs()
	for b.Loop() {
		semantics.Snapshot(root)
	}
}
