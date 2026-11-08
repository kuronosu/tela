package core

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
	"github.com/kuronosu/tela/internal/platform"
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
	{
		// ADR-096: the snapshot prints none of the widget props; only
		// aria-controls counts, as a global attribute that keeps the
		// implicit role of an element with role presentation.
		name: "widget patterns",
		root: func() Node { return Component(widgetScreen) },
		want: "- button \"Options\" [expanded]\n- menu:\n  - menuitem \"Copy\"\n- listbox:\n  - option \"One\" [selected]\n  - option \"Two\"\n" +
			"- combobox \"Fruit\"\n- dialog \"Confirm\":\n  - button \"OK\"\n- heading \"Kept\" [level=1]\n- text: Gone",
		html: `<div><button aria-haspopup="menu" aria-expanded="true" aria-controls="m">Options</button><div id="m" role="menu" aria-orientation="vertical"><div role="menuitem">Copy</div></div>` +
			`<div role="listbox" aria-multiselectable="true" aria-orientation="horizontal" aria-activedescendant="o1" tabindex="0"><div id="o1" role="option" aria-selected="true">One</div><div role="option">Two</div></div>` +
			`<div role="combobox" aria-autocomplete="list" aria-haspopup="listbox" aria-expanded="false" aria-label="Fruit" tabindex="0"></div>` +
			`<div role="dialog" aria-modal="true" aria-label="Confirm"><button>OK</button></div>` +
			`<h1 role="presentation" aria-controls="m">Kept</h1><h2 role="presentation" aria-haspopup="true">Gone</h2></div>`,
	},
	{
		// ADR-131: [invalid] after [expanded], on the roles that support
		// it, from the aria-invalid attribute alone (not a field's own
		// validity); aria-sort is not printed.
		name: "invalid and sort",
		root: func() Node {
			return div(
				div(Role("textbox"), AriaLabel("Email"), AriaInvalid(true)),
				div(Role("checkbox"), AriaChecked(true), AriaInvalid(true), Text("Terms")),
				button(AriaInvalid(true), AriaExpanded(true), Text("Menu")),
				div(Role("combobox"), AriaLabel("Fruit"), AriaExpanded(true), AriaInvalid(true), TabIndex(0)),
				div(Role("textbox"), AriaLabel("Name"), AriaInvalid(false)),
				input(FieldType("email"), AriaLabel("Mail"), DefaultValue("bad")),
				div(Role("grid"), div(Role("row"),
					div(Role("columnheader"), AriaSort("ascending"), AriaInvalid(true), Text("Name")),
					div(Role("columnheader"), AriaSort("descending"), Text("Size")))),
			)
		},
		want: "- textbox \"Email\" [invalid]\n- checkbox \"Terms\" [checked] [invalid]\n- button \"Menu\" [expanded]\n" +
			"- combobox \"Fruit\" [expanded] [invalid]\n- textbox \"Name\"\n- textbox \"Mail\": bad\n" +
			"- grid:\n  - row \"Name Size\":\n    - columnheader \"Name\" [invalid]\n    - columnheader \"Size\"",
		html: `<div><div role="textbox" aria-label="Email" aria-invalid="true"></div><div role="checkbox" aria-checked="true" aria-invalid="true">Terms</div>` +
			`<button aria-invalid="true" aria-expanded="true">Menu</button><div role="combobox" aria-label="Fruit" aria-expanded="true" aria-invalid="true" tabindex="0"></div>` +
			`<div role="textbox" aria-label="Name" aria-invalid="false"></div><input type="email" aria-label="Mail" value="bad">` +
			`<div role="grid"><div role="row"><div role="columnheader" aria-sort="ascending" aria-invalid="true">Name</div><div role="columnheader" aria-sort="descending">Size</div></div></div></div>`,
	},
}

func widgetScreen() Node {
	menu, opt := UseElementRef(), UseElementRef()
	return div(
		button(AriaHasPopup("menu"), AriaExpanded(true), AriaControls(menu), Text("Options")),
		div(AttachRef(menu), Role("menu"), AriaOrientation("vertical"), div(Role("menuitem"), Text("Copy"))),
		div(Role("listbox"), AriaMultiSelectable(true), AriaOrientation("horizontal"), AriaActiveDescendant(opt), TabIndex(0),
			div(AttachRef(opt), Role("option"), AriaSelected(true), Text("One")), div(Role("option"), Text("Two"))),
		div(Role("combobox"), AriaAutoComplete("list"), AriaHasPopup("listbox"), AriaExpanded(false), AriaLabel("Fruit"), TabIndex(0)),
		div(Role("dialog"), AriaModal(true), AriaLabel("Confirm"), button(Text("OK"))),
		h1(Role("presentation"), AriaControls(menu), Text("Kept")),
		h2(Role("presentation"), AriaHasPopup("true"), Text("Gone")),
	)
}

// TestWidgetSemantics: the widget props in the semantics tree.
func TestWidgetSemantics(t *testing.T) {
	tr, _ := snapshotOf(t, Component(widgetScreen))
	btn := mustNode(t, tr, "button", "Options")
	menu := mustNode(t, tr, "menu", "")
	if btn.HasPopup != "menu" || btn.Controls != menu.ID || menu.Orientation != "vertical" {
		t.Errorf("menu button %+v, menu %+v", btn, menu)
	}
	lb := mustNode(t, tr, "listbox", "")
	one := mustNode(t, tr, "option", "One")
	if !lb.MultiSelectable || lb.Orientation != "horizontal" || lb.ActiveDescendant != one.ID {
		t.Errorf("listbox %+v", lb)
	}
	cb := mustNode(t, tr, "combobox", "Fruit")
	if cb.AutoComplete != "list" || cb.HasPopup != "listbox" {
		t.Errorf("combobox %+v", cb)
	}
	if d := mustNode(t, tr, "dialog", "Confirm"); !d.Modal {
		t.Errorf("dialog %+v", d)
	}
}

// TestWidgetPropValues: the string props take their ARIA tokens ASCII
// case-insensitively, and unknown values panic in dev mode with the
// closest one.
func TestWidgetPropValues(t *testing.T) {
	tr, _ := snapshotOf(t, div(
		div(Key("a"), Role("button"), AriaHasPopup("Dialog")),
		div(Key("b"), Role("tablist"), AriaOrientation("undefined")),
		div(Key("c"), Role("combobox"), AriaAutoComplete("none"), AriaHasPopup("false"))))
	if w := keyed(tr, "a").node.d.aria.w; w.hasPopup != "dialog" {
		t.Errorf("AriaHasPopup(\"Dialog\"): %q", w.hasPopup)
	}
	if w := keyed(tr, "b").node.d.aria.w; w.orientation != "" {
		t.Errorf("AriaOrientation(\"undefined\"): %q", w.orientation)
	}
	if w := keyed(tr, "c").node.d.aria.w; w.autoComplete != "" || w.hasPopup != "" {
		t.Errorf("AriaAutoComplete(\"none\"), AriaHasPopup(\"false\"): %+v", w)
	}
	devTree(t, div(), nil) // dev mode on
	for _, c := range []struct {
		f    func()
		want string
	}{
		{func() { AriaHasPopup("listbx") }, `AriaHasPopup("listbx"): unknown value; did you mean "listbox"?`},
		{func() { AriaOrientation("vertcal") }, `did you mean "vertical"?`},
		{func() { AriaAutoComplete("lists") }, `did you mean "list"?`},
		{func() { AriaHasPopup("popup") }, `it takes "menu", "listbox"`},
	} {
		if msg := panicMessage(c.f); !strings.Contains(msg, c.want) {
			t.Errorf("panic %q, want %q", msg, c.want)
		}
	}
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
	tr.setScroll(sc.box.scr, gfx.Point{Y: 100}) // painted offsets follow at the next paint
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

func labelledByManyScreen() Node {
	value, label, self, missing := UseElementRef(), UseElementRef(), UseElementRef(), UseElementRef()
	return div(
		span(AttachRef(label), Text("Favorite fruit")),
		button(AriaLabelledBy(value, label), span(AttachRef(value), Text("Apple"))),
		button(AriaLabel("Pick"), AttachRef(self), AriaLabelledBy(value, self), span(Text("ignored"))),
		button(AriaLabelledBy(missing, label), Text("content")),
	)
}

// TestAriaLabelledByMany: several references name an element with the text
// of each mounted one, in the order given and joined by a space
// (aria-labelledby="value label"); a reference to the element itself
// gives its AriaLabel, and one that is not mounted gives nothing.
func TestAriaLabelledByMany(t *testing.T) {
	_, got := snapshotOf(t, Component(labelledByManyScreen))
	want := "- text: Favorite fruit\n" +
		"- button \"Apple Favorite fruit\": Apple\n" +
		"- button \"Apple Pick\": ignored\n" +
		"- button \"Favorite fruit\": content"
	if got != want {
		t.Errorf("snapshot\n%s\nwant\n%s", got, want)
	}
}

// TestComboboxFieldValue: a text field with the combobox role exposes its
// text, as Playwright prints an `input role="combobox"`.
func TestComboboxFieldValue(t *testing.T) {
	_, got := snapshotOf(t, div(
		input(Role("combobox"), AriaLabel("Fruit"), AriaExpanded(true), DefaultValue("App")),
		input(Role("combobox"), AriaLabel("Empty"))))
	want := "- combobox \"Fruit\" [expanded]: App\n- combobox \"Empty\""
	if got != want {
		t.Errorf("snapshot\n%s\nwant\n%s", got, want)
	}
}

// A text unmounted since the last Semantics keeps its parent: an action on
// its node is ErrNodeGone, not an action on the parent that is still
// mounted.
func TestSemanticsActionOnUnmountedText(t *testing.T) {
	clicks := 0
	tr, _ := snapshotOf(t, div(button(Key("go"), On("click", func(MouseEvent) { clicks++ }), Text("Go"))))
	var text semantics.NodeID
	var walk func(n *semantics.Node)
	walk = func(n *semantics.Node) {
		if n.Kind == semantics.KindText && n.Text == "Go" {
			text = n.ID
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(tr.Semantics())
	if text == 0 {
		t.Fatal("no text Go in the semantics tree")
	}
	tr.Mount(div(button(Key("go"), On("click", func(MouseEvent) { clicks++ }))))
	tr.Frame()
	if err := tr.ClickNode(text); !errors.Is(err, ErrNodeGone) || clicks != 0 {
		t.Errorf("ClickNode(unmounted text) = %v with %d clicks; want ErrNodeGone and none", err, clicks)
	}
}

// ---------------------------------------------------------------------
// The fields and actions of the OS accessibility bridges (ADR-246).

// TestSemanticsLiveRegions: Live, Atomic and LiveRoot, explicit and
// implicit by role, on elements and texts.
func TestSemanticsLiveRegions(t *testing.T) {
	tr, _ := snapshotOf(t, div(
		div(Role("alert"), AriaLabel("A"), span(Text("Saved"))),
		div(Role("status"), AriaLabel("S"), AriaLive("assertive"), AriaAtomic(false)),
		div(Role("log"), AriaLabel("L")),
		div(Role("region"), AriaLabel("R"), AriaLive("Polite"),
			div(Role("group"), AriaLabel("inner"), Text("x")),
			div(Role("group"), AriaLabel("off"), AriaLive("off"), div(Role("group"), AriaLabel("in off"))),
			div(Role("timer"), AriaLabel("T"))),
		div(Role("alert"), AriaLabel("quiet"), AriaLive("off")),
		div(Role("group"), AriaLabel("plain"))))
	sem := tr.Semantics()
	for _, c := range []struct {
		role, name string
		live       string
		atomic     bool
		root       string // name of the region root, "" for none
	}{
		{"alert", "A", "assertive", true, "A"},
		{"status", "S", "assertive", false, "S"},
		{"log", "L", "polite", false, "L"},
		{"region", "R", "polite", false, "R"},
		{"group", "inner", "", false, "R"},
		{"group", "off", "", false, ""},
		{"group", "in off", "", false, ""},
		{"timer", "T", "", false, ""},
		{"alert", "quiet", "", true, ""},
		{"group", "plain", "", false, ""},
	} {
		n := mustNode(t, tr, c.role, c.name)
		var root semantics.NodeID
		if c.root != "" {
			root = findNode(sem, map[string]string{"A": "alert", "S": "status", "L": "log", "R": "region"}[c.root], c.root).ID
		}
		if n.Live != c.live || n.Atomic != c.atomic || n.LiveRoot != root {
			t.Errorf("%s %q: live %q atomic %v root %d; want %q %v %d", c.role, c.name, n.Live, n.Atomic, n.LiveRoot, c.live, c.atomic, root)
		}
	}
	alert := mustNode(t, tr, "alert", "A")
	var text *semantics.Node
	alert.Walk(func(n *semantics.Node) bool {
		if n.Kind == semantics.KindText {
			text = n
		}
		return true
	})
	if text == nil || text.LiveRoot != alert.ID {
		t.Errorf("the text of an alert: %+v, want LiveRoot %d", text, alert.ID)
	}
}

// TestSemanticsNewPropChecks: AriaLive takes its tokens like the other
// token props, and AriaKeyShortcuts rejects empty keys and aliases in dev
// mode.
func TestSemanticsNewPropChecks(t *testing.T) {
	tr, _ := snapshotOf(t, div(button(AriaKeyShortcuts("  Control+S   Meta+S "), Text("Save"))))
	if n := mustNode(t, tr, "button", "Save"); n.KeyShortcuts != "Control+S Meta+S" {
		t.Errorf("KeyShortcuts %q", n.KeyShortcuts)
	}
	devTree(t, div(), nil) // dev mode on
	for _, c := range []struct {
		f    func()
		want string
	}{
		{func() { AriaLive("politely") }, `AriaLive("politely"): unknown value; did you mean "polite"?`},
		{func() { AriaKeyShortcuts("Ctrl+S") }, `"Ctrl" is not a UI Events key value; did you mean "Control"?`},
		{func() { AriaKeyShortcuts("Control+") }, `the chord "Control+" has an empty key`},
		{func() { AriaKeyShortcuts("Shift++") }, `write the + key as Plus`},
		{func() { AriaLabelFn(nil) }, `AriaLabelFn(nil)`},
	} {
		if msg := panicMessage(c.f); !strings.Contains(msg, c.want) {
			t.Errorf("panic %q, want %q", msg, c.want)
		}
	}
	for _, ok := range []string{"Shift+Space", "Alt+Shift+N", "Control+Plus", "F5", ""} {
		if msg := CheckKeyShortcuts(ok); msg != "" {
			t.Errorf("CheckKeyShortcuts(%q) = %q", ok, msg)
		}
	}
	if CheckAriaToken("AriaLive", "assertive") != "" || CheckAriaToken("AriaLive", "loud") == "" {
		t.Error("CheckAriaToken does not check AriaLive")
	}
}

// posOf returns "pos/size" of the node with role and name.
func posOf(t *testing.T, tr *Tree, role, name string) string {
	t.Helper()
	n := mustNode(t, tr, role, name)
	return strconv.Itoa(n.PosInSet) + "/" + strconv.Itoa(n.SetSize)
}

// TestSemanticsSetPositions: PosInSet and SetSize of the items of sets,
// implicit as Chromium computes them and explicit.
func TestSemanticsSetPositions(t *testing.T) {
	opt := func(name string, ps ...Child) Node {
		return div(append([]Child{Role("option"), Text(name)}, ps...)...)
	}
	tr, _ := snapshotOf(t, div(
		div(Role("listbox"), AriaLabel("Fruit"),
			opt("Apple"), div(opt("Banana")), opt("Hidden", Class("hidden")), opt("Cherry"),
			div(Role("group"), AriaLabel("More"), opt("Date"), opt("Elder"))),
		div(Role("menu"), AriaLabel("Edit"),
			div(Role("menuitem"), Text("Cut")), div(Role("separator")),
			div(Role("menuitemcheckbox"), AriaChecked(true), Text("Wrap")),
			div(Role("group"), div(Role("menuitemradio"), AriaChecked(true), Text("Left")), div(Role("menuitemradio"), AriaChecked(false), Text("Right"))),
			div(Role("menuitem"), Text("Paste"))),
		div(Role("tablist"), div(Role("tab"), Text("One")), div(Role("tab"), Text("Two"))),
		div(Role("radiogroup"), AriaLabel("Size"), div(Role("radio"), Text("S")), div(Role("radio"), Text("M")), div(Role("radio"), Text("L"))),
		div(Role("tree"), AriaLabel("Files"),
			div(Role("treeitem"), Text("src"), div(Role("group"), div(Role("treeitem"), Text("a.go")), div(Role("treeitem"), Text("b.go")))),
			div(Role("treeitem"), Text("docs"))),
		div(Role("list"), AriaLabel("Paged"), AriaSetSize(100),
			div(Role("listitem"), AriaPosInSet(41), AriaLabel("Item 41")), div(Role("listitem"), AriaLabel("Item 42"))),
		div(Role("list"), AriaLabel("Own"), div(Role("listitem"), AriaSetSize(-1), AriaLabel("Item a")), div(Role("listitem"), AriaLabel("Item b"))),
		opt("Loose")))
	for _, c := range []struct{ role, name, want string }{
		{"option", "Apple", "1/3"}, {"option", "Banana", "2/3"}, {"option", "Cherry", "3/3"},
		{"option", "Date", "1/2"}, {"option", "Elder", "2/2"},
		{"menuitem", "Cut", "1/3"}, {"menuitemcheckbox", "Wrap", "2/3"}, {"menuitem", "Paste", "3/3"},
		{"menuitemradio", "Left", "1/2"}, {"menuitemradio", "Right", "2/2"},
		{"tab", "One", "1/2"}, {"tab", "Two", "2/2"},
		{"radio", "M", "2/3"},
		{"treeitem", "src", "1/2"}, {"treeitem", "docs", "2/2"}, {"treeitem", "b.go", "2/2"},
		{"listitem", "Item 41", "41/100"}, {"listitem", "Item 42", "42/100"},
		{"listitem", "Item a", "1/-1"}, {"listitem", "Item b", "2/2"},
		{"option", "Loose", "0/0"},
		{"listbox", "Fruit", "0/0"}, {"list", "Paged", "0/100"},
	} {
		if got := posOf(t, tr, c.role, c.name); got != c.want {
			t.Errorf("%s %q: %s, want %s", c.role, c.name, got, c.want)
		}
	}
	if h := findNode(tr.Semantics(), "option", "Hidden"); h == nil || h.PosInSet != 0 {
		t.Errorf("a hidden option: %+v", h)
	}
}

// TestSemanticsTextFields: the placeholder, the protected state of a
// password and the caret and selection in code points.
func TestSemanticsTextFields(t *testing.T) {
	tr, _ := snapshotOf(t, div(
		input(Key("pw"), FieldType("password"), AriaLabel("Password"), Placeholder("At least 8 characters"), DefaultValue("hé😀")),
		textarea(Key("bio"), AriaLabel("Bio"), DefaultValue("aé😀b")),
		input(Key("ro"), AriaLabel("Code"), ReadOnly(true), DefaultValue("XY")),
		div(Role("textbox"), AriaLabel("Div"), Text("text"))))
	pw := mustNode(t, tr, "textbox", "Password")
	if !pw.Has(semantics.StateProtected) || pw.Placeholder != "At least 8 characters" || pw.Value != "•••" {
		t.Errorf("password %+v", pw)
	}
	if pw.Selection == nil || !pw.Can(semantics.ActionSetSelection) {
		t.Fatalf("password selection %+v", pw.Selection)
	}
	bio := mustNode(t, tr, "textbox", "Bio")
	if bio.Has(semantics.StateProtected) || bio.Selection == nil {
		t.Fatalf("textarea %+v", bio)
	}
	// Select "é😀" backwards: code points 3 to 1 (bytes 7 to 1).
	if err := tr.SetTextSelectionNode(bio.ID, 3, 1); err != nil {
		t.Fatal(err)
	}
	f := keyed(tr, "bio").box.fld
	if s, e, back := f.ed.SelectionRange(); s != 1 || e != 7 || !back {
		t.Errorf("selection bytes %d..%d backward %v, want 1..7 backward", s, e, back)
	}
	tr.Frame()
	if got := *mustNode(t, tr, "textbox", "Bio").Selection; got != (semantics.TextSelection{Anchor: 3, Focus: 1}) {
		t.Errorf("Selection %+v, want anchor 3 focus 1", got)
	}
	// Past the end clamps; the password field's offsets count bullets.
	if err := tr.SetTextSelectionNode(pw.ID, 2, 99); err != nil {
		t.Fatal(err)
	}
	if s, e, _ := keyed(tr, "pw").box.fld.ed.SelectionRange(); s != 3 || e != 7 {
		t.Errorf("password selection bytes %d..%d, want 3..7", s, e)
	}
	tr.Frame()
	if got := *mustNode(t, tr, "textbox", "Password").Selection; got != (semantics.TextSelection{Anchor: 2, Focus: 3}) {
		t.Errorf("password Selection %+v", got)
	}
	ro := mustNode(t, tr, "textbox", "Code")
	if ro.Selection == nil || !ro.Can(semantics.ActionSetSelection) || ro.Can(semantics.ActionSetValue) {
		t.Errorf("read-only field %+v", ro)
	}
	d := mustNode(t, tr, "textbox", "Div")
	if d.Selection != nil || d.Can(semantics.ActionSetSelection) {
		t.Errorf("a div textbox has text field data: %+v", d)
	}
	if err := tr.SetTextSelectionNode(d.ID, 0, 1); !errors.Is(err, ErrNoAction) {
		t.Errorf("SetTextSelectionNode(div) = %v, want ErrNoAction", err)
	}
}

// A field whose role exposes a number, not its text, has no selection to
// set: its Value's offsets are not the field's.
func TestSetTextSelectionNodeSpinbutton(t *testing.T) {
	tr, _ := snapshotOf(t, div(input(Role("spinbutton"), AriaLabel("Qty"), AriaValueNow(3), DefaultValue("3 items"))))
	n := mustNode(t, tr, "spinbutton", "Qty")
	if n.Selection != nil || n.Can(semantics.ActionSetSelection) {
		t.Errorf("a spinbutton field has a selection: %+v", n)
	}
	if err := tr.SetTextSelectionNode(n.ID, 0, 1); !errors.Is(err, ErrNoAction) {
		t.Errorf("SetTextSelectionNode(spinbutton) = %v, want ErrNoAction", err)
	}
}

// The actions the tree withholds from an AriaDisabled element are refused
// when called directly, as for a natively disabled one.
func TestAriaDisabledActions(t *testing.T) {
	var keys []string
	clicks := 0
	tr, _ := snapshotOf(t, div(
		Component(func() Node { return stepSlider("S", &keys, AriaDisabled(true)) }),
		div(AriaDisabled(true),
			button(AriaExpanded(false), On("click", func(MouseEvent) { clicks++ }), Text("More")),
			div(TabIndex(0), AriaLabel("Area"), Role("region"), On("contextmenu", func(MouseEvent) { clicks++ })))))
	if n := mustNode(t, tr, "slider", "S"); n.Can(semantics.ActionIncrement) {
		t.Errorf("an AriaDisabled slider can increment: %+v", n)
	} else if err := tr.IncrementNode(n.ID); !errors.Is(err, ErrNodeDisabled) || len(keys) != 0 {
		t.Errorf("IncrementNode(AriaDisabled) = %v with keys %v, want ErrNodeDisabled", err, keys)
	}
	if err := tr.ExpandNode(mustNode(t, tr, "button", "More").ID); !errors.Is(err, ErrNodeDisabled) || clicks != 0 {
		t.Errorf("ExpandNode(in an AriaDisabled group) = %v with %d clicks", err, clicks)
	}
	if err := tr.ShowMenuNode(mustNode(t, tr, "region", "Area").ID); !errors.Is(err, ErrNodeDisabled) || clicks != 0 {
		t.Errorf("ShowMenuNode(in an AriaDisabled group) = %v with %d clicks", err, clicks)
	}
}

// TestSemanticsWidgetActions: the action bits of range widgets, expandable
// elements and context-menu areas.
func TestSemanticsWidgetActions(t *testing.T) {
	tr, _ := snapshotOf(t, div(
		div(Role("slider"), AriaLabel("Volume"), TabIndex(0)),
		div(Role("slider"), AriaLabel("Locked"), TabIndex(0), AriaReadOnly(true)),
		div(Role("slider"), AriaLabel("Static")),
		div(Role("spinbutton"), AriaLabel("Off"), TabIndex(0), AriaDisabled(true)),
		button(AriaExpanded(false), Text("Closed")),
		button(AriaExpanded(true), Text("Open")),
		button(AriaExpanded(true), Disabled(true), Text("Dead")),
		div(Role("region"), AriaLabel("Area"), On("contextmenu", func(MouseEvent) {})),
		button(Text("Plain"))))
	step := semantics.ActionIncrement | semantics.ActionDecrement
	for _, c := range []struct {
		role, name string
		has, not   semantics.Action
	}{
		{"slider", "Volume", step, 0},
		{"slider", "Locked", 0, step},
		{"slider", "Static", 0, step},
		{"spinbutton", "Off", 0, step},
		{"button", "Closed", semantics.ActionExpand, semantics.ActionCollapse},
		{"button", "Open", semantics.ActionCollapse, semantics.ActionExpand},
		{"button", "Dead", 0, semantics.ActionCollapse | semantics.ActionExpand},
		{"region", "Area", semantics.ActionShowMenu, 0},
		{"button", "Plain", 0, semantics.ActionShowMenu | semantics.ActionExpand | step},
	} {
		n := mustNode(t, tr, c.role, c.name)
		if !n.Can(c.has) || n.Actions&c.not != 0 {
			t.Errorf("%s %q: actions %b, want %b and not %b", c.role, c.name, n.Actions, c.has, c.not)
		}
	}
}

// stepSlider is a slider that records the keys it gets and moves by one
// per arrow (ArrowUp and ArrowRight increase, mirrored in RTL like aria's).
func stepSlider(name string, keys *[]string, ps ...Child) Node {
	v, set := UseState(5)
	return div(append([]Child{Role("slider"), AriaLabel(name), TabIndex(0), AriaValueNow(float64(v)),
		On("keydown", func(e KeyboardEvent) {
			*keys = append(*keys, string(e.Key))
			switch e.Key {
			case "ArrowUp", "ArrowRight":
				set.Set(v + 1)
			case "ArrowDown", "ArrowLeft":
				set.Set(v - 1)
			}
			e.PreventDefault()
		})}, ps...)...)
}

func TestIncrementDecrementNode(t *testing.T) {
	var keys []string
	tr, _ := snapshotOf(t, div(
		Component(func() Node { return stepSlider("H", &keys) }),
		Component(func() Node { return stepSlider("V", &keys, AriaOrientation("vertical")) }),
		Component(func() Node { return stepSlider("R", &keys, Dir("rtl")) }),
		Component(func() Node { return stepSlider("RO", &keys, AriaReadOnly(true)) }),
		button(Text("B"))))
	for _, c := range []struct {
		name string
		up   bool
		key  string
	}{
		{"H", true, "ArrowRight"}, {"H", false, "ArrowLeft"},
		{"V", true, "ArrowUp"}, {"V", false, "ArrowDown"},
		{"R", true, "ArrowLeft"}, {"R", false, "ArrowRight"},
	} {
		keys = keys[:0]
		n := mustNode(t, tr, "slider", c.name)
		f := tr.DecrementNode
		if c.up {
			f = tr.IncrementNode
		}
		if err := f(n.ID); err != nil {
			t.Fatal(err)
		}
		if len(keys) != 1 || keys[0] != c.key {
			t.Errorf("%s up=%v: keys %v, want [%s]", c.name, c.up, keys, c.key)
		}
		if el := tr.liveElement(n.ID); tr.foc.el != el {
			t.Errorf("%s: not focused", c.name)
		}
	}
	tr.Frame()
	// H: +1 -1; R: in RTL ArrowLeft decreased, ArrowRight increased.
	if n := mustNode(t, tr, "slider", "H"); n.Value != "5" {
		t.Errorf("H value %q", n.Value)
	}
	if err := tr.IncrementNode(mustNode(t, tr, "slider", "V").ID); err != nil {
		t.Fatal(err)
	}
	tr.Frame()
	if n := mustNode(t, tr, "slider", "V"); n.Value != "6" {
		t.Errorf("V value after an increment %q, want 6", n.Value)
	}
	if err := tr.IncrementNode(mustNode(t, tr, "slider", "RO").ID); !errors.Is(err, ErrNoAction) {
		t.Errorf("IncrementNode(read-only) = %v", err)
	}
	if err := tr.IncrementNode(mustNode(t, tr, "button", "B").ID); !errors.Is(err, ErrNoAction) {
		t.Errorf("IncrementNode(button) = %v", err)
	}
}

func TestExpandCollapseNode(t *testing.T) {
	clicks := 0
	disclosure := func() Node {
		open, set := UseState(false)
		return button(AriaExpanded(open), On("click", func(MouseEvent) { clicks++; set.Set(!open) }), Text("More"))
	}
	tr, _ := snapshotOf(t, div(Component(disclosure), button(Text("Plain"))))
	id := mustNode(t, tr, "button", "More").ID
	if err := tr.CollapseNode(id); err != nil || clicks != 0 {
		t.Errorf("CollapseNode(collapsed) = %v with %d clicks; want nil and none", err, clicks)
	}
	if err := tr.ExpandNode(id); err != nil || clicks != 1 {
		t.Fatalf("ExpandNode = %v with %d clicks", err, clicks)
	}
	tr.Frame()
	if n := mustNode(t, tr, "button", "More"); n.Expanded != semantics.ToggleTrue || !n.Can(semantics.ActionCollapse) {
		t.Errorf("after ExpandNode %+v", n)
	}
	if err := tr.ExpandNode(id); err != nil || clicks != 1 {
		t.Errorf("ExpandNode(expanded) = %v with %d clicks", err, clicks)
	}
	if err := tr.CollapseNode(id); err != nil || clicks != 2 {
		t.Errorf("CollapseNode = %v with %d clicks", err, clicks)
	}
	if err := tr.ExpandNode(mustNode(t, tr, "button", "Plain").ID); !errors.Is(err, ErrNoAction) {
		t.Errorf("ExpandNode(plain button) = %v", err)
	}
}

func TestShowMenuNode(t *testing.T) {
	var got []string
	tr, _ := snapshotOf(t, div(
		div(Role("region"), AriaLabel("Plain"), On("contextmenu", func(e MouseEvent) {
			got = append(got, fmt.Sprintf("menu %v %.0f,%.0f", e.Button == MouseButtonRight, e.X, e.Y))
		}), Class("h-10 w-20")),
		div(Role("region"), AriaLabel("Keyed"), TabIndex(0),
			On("keydown", func(e KeyboardEvent) {
				if e.Key == KeyValue(platform.KeyContextMenu) {
					got = append(got, "key")
					e.PreventDefault()
				}
			}),
			On("contextmenu", func(MouseEvent) { got = append(got, "menu") })),
		button(Text("None"))))
	if err := tr.ShowMenuNode(mustNode(t, tr, "region", "Plain").ID); err != nil {
		t.Fatal(err)
	}
	if err := tr.ShowMenuNode(mustNode(t, tr, "region", "Keyed").ID); err != nil {
		t.Fatal(err)
	}
	if want := []string{"menu true 40,20", "key"}; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("events %q, want %q", got, want)
	}
	if el := tr.liveElement(mustNode(t, tr, "region", "Keyed").ID); tr.foc.el != el {
		t.Error("ShowMenuNode did not focus the focusable area")
	}
	if err := tr.ShowMenuNode(mustNode(t, tr, "button", "None").ID); !errors.Is(err, ErrNoAction) {
		t.Errorf("ShowMenuNode(button) = %v", err)
	}
}

func TestScrollNodeBy(t *testing.T) {
	tr, _ := snapshotOf(t, div(
		div(Key("list"), Role("list"), AriaLabel("Rows"), Class("h-20 overflow-y-auto"), div(Class("h-96"))),
		button(Text("B"))))
	n := mustNode(t, tr, "list", "Rows")
	if err := tr.ScrollNodeBy(n.ID, 0, 30); err != nil {
		t.Fatal(err)
	}
	if err := tr.ScrollNodeBy(n.ID, 0, 15); err != nil {
		t.Fatal(err)
	}
	if y := keyed(tr, "list").box.scr.pos.Y; y != 45 {
		t.Errorf("scrolled to %v, want 45", y)
	}
	if err := tr.ScrollNodeBy(mustNode(t, tr, "button", "B").ID, 0, 10); !errors.Is(err, ErrNoAction) {
		t.Errorf("ScrollNodeBy(button) = %v", err)
	}
}

// TestAriaLabelFn: the deferred name is read when the semantics tree is
// built; a change of a Value it read rebuilds nothing, requests no frame,
// and makes the next tree fresh, telling the listener once.
func TestAriaLabelFn(t *testing.T) {
	var pct *Value[int]
	builds := 0
	tr, _ := snapshotOf(t, Component(func() Node {
		builds++
		pct = UseValue(10)
		return div(div(Role("progressbar"), AriaLabelFn(func() string { return fmt.Sprintf("Upload %d%%", pct.Get()) })))
	}))
	mustNode(t, tr, "progressbar", "Upload 10%")
	told := 0
	tr.OnSemanticsStale(func() { told++ })
	if tr.SemanticsStale() {
		t.Error("stale before any change")
	}
	pct.Set(55)
	if tr.ApplyUpdates() {
		t.Error("a Value read only by AriaLabelFn requested a frame")
	}
	if !tr.SemanticsStale() || told != 1 {
		t.Errorf("stale %v, told %d; want true, 1", tr.SemanticsStale(), told)
	}
	pct.Set(60)
	tr.ApplyUpdates()
	if told != 1 {
		t.Errorf("told %d times before a new tree was built", told)
	}
	mustNode(t, tr, "progressbar", "Upload 60%")
	if builds != 1 {
		t.Errorf("%d builds, want 1", builds)
	}
	pct.Set(70)
	tr.ApplyUpdates()
	if told != 2 {
		t.Errorf("told %d times after a new tree, want 2", told)
	}
	// AriaLabel replaces AriaLabelFn and drops its subscriptions.
	tr.Mount(div(div(Role("progressbar"), AriaLabelFn(func() string { return "x" }), AriaLabel("Fixed"))))
	tr.Frame()
	mustNode(t, tr, "progressbar", "Fixed")
}
