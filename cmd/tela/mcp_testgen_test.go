package main

import (
	"bytes"
	"go/format"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

func TestTestFuncName(t *testing.T) {
	for in, want := range map[string]string{
		"checkout flow":    "TestCheckoutFlow",
		"TestCheckoutFlow": "TestCheckoutFlow",
		"login":            "TestLogin",
		"save-and_close":   "TestSaveAndClose",
		"  two words ":     "TestTwoWords",
		"añadir":           "TestAñadir",
		"v2 flow":          "TestV2Flow",
	} {
		got, err := testFuncName(in)
		if err != nil || got != want {
			t.Errorf("testFuncName(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for in, want := range map[string]string{
		"":         "needs a name",
		"Test":     "needs a name",
		"a/b":      `has '/' in it`,
		"1st flow": "starts with a digit",
	} {
		if _, err := testFuncName(in); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("testFuncName(%q): error = %v, want %q", in, err, want)
		}
	}
}

func TestTestFileName(t *testing.T) {
	for in, want := range map[string]string{
		"TestCheckoutFlow": "checkout_flow_test.go",
		"TestLogin":        "login_test.go",
		"TestV2Flow":       "v2_flow_test.go",
	} {
		if got := testFileName(in); got != want {
			t.Errorf("testFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func intp(n int) *int { return &n }

func TestLocGoExpr(t *testing.T) {
	stable := mcpLoc{Role: "button", Name: "Save", Nth: intp(1)}
	for _, tc := range []struct {
		loc  mcpLoc
		want string
	}{
		// A ref stands for its stable form, never its node identity.
		{mcpLoc{NodeID: 42, Ref: "e7", Stable: &mcpLoc{Role: "button", Name: "Save"}}, `telatest.ByRole("button", "Save")`},
		{mcpLoc{NodeID: 42, Ref: "e7", Stable: &stable}, `telatest.ByRole("button", "Save").Nth(1)`},
		{mcpLoc{Role: "textbox", Name: `Say "hi"`}, `telatest.ByRole("textbox", "Say \"hi\"")`},
		{mcpLoc{Role: "heading"}, `telatest.ByRole("heading", "")`},
		{mcpLoc{TestID: "save"}, `telatest.ByTestID("save")`},
		{mcpLoc{TestID: "row", Nth: intp(2)}, `telatest.ByTestID("row").Nth(2)`},
		{mcpLoc{Text: "Saved"}, `telatest.ByText("Saved")`},
		{mcpLoc{Text: "Saved", Exact: true, Nth: intp(0)}, `telatest.ByText("Saved", telatest.Exact()).Nth(0)`},
	} {
		if got := tc.loc.goExpr(); got != tc.want {
			t.Errorf("%+v: %s, want %s", tc.loc, got, tc.want)
		}
		if strings.Contains(tc.want, "NodeId") {
			t.Errorf("a node identity in %s", tc.want)
		}
	}
}

func TestPlainSnapshot(t *testing.T) {
	in := "- heading \"Hi\" [level=1] [src=app.go:12]\n- button \"Go\" [disabled] [src=app.go:14]: label\n- text: plain"
	want := "- heading \"Hi\" [level=1]\n- button \"Go\" [disabled]: label\n- text: plain"
	if got := plainSnapshot(in); got != want {
		t.Errorf("plainSnapshot:\n%s\nwant\n%s", got, want)
	}
	if got := plainSnapshot(want); got != want {
		t.Errorf("a snapshot without sources changed:\n%s", got)
	}
}

func TestExpectations(t *testing.T) {
	for _, tc := range []struct {
		name          string
		before, after string
		want          []string
	}{
		{"nothing changed", `- button "Go"`, `- button "Go"`, nil},
		{
			"a renamed button and a new paragraph",
			"- heading \"T\" [level=1]\n- 'button \"Count: 0\"'",
			"- heading \"T\" [level=1]\n- paragraph: Hello, Ana!\n- 'button \"Count: 1\"'",
			[]string{`app.ExpectText("Hello, Ana!")`, `app.ExpectRole("button", "Count: 1")`},
		},
		{
			"a field takes a value",
			"- textbox \"Name\"",
			"- textbox \"Name\": Ana",
			[]string{`app.ExpectValue(telatest.ByRole("textbox", "Name"), "Ana")`},
		},
		{
			"a state flips on and off",
			"- checkbox \"Agree\"\n- switch \"Wi-Fi\" [checked]",
			"- checkbox \"Agree\" [checked]\n- switch \"Wi-Fi\"",
			[]string{`app.Expect(telatest.ByRole("checkbox", "Agree"), telatest.Checked(true))`, `app.Expect(telatest.ByRole("switch", "Wi-Fi"), telatest.Checked(false))`},
		},
		{
			"an element and a text go away",
			"- button \"Remove\"\n- paragraph: Saved\n- heading \"Keep\"",
			"- heading \"Keep\"",
			[]string{`app.ExpectCount(telatest.ByRole("button", "Remove"), 0)`, `app.ExpectNoText("Saved")`},
		},
		{
			"a text that is still shown elsewhere is not checked gone",
			"- paragraph: Saved\n- paragraph: Other",
			"- paragraph: Other\n- button \"Saved\"",
			[]string{`app.ExpectRole("button", "Saved")`},
		},
		{
			"a new element among others like it gets an Nth",
			"- button \"Remove\"",
			"- button \"Remove\"\n- button \"Remove\" [disabled]",
			[]string{`app.Expect(telatest.ByRole("button", "Remove").Nth(1), telatest.Disabled(true))`},
		},
		{
			"a new expanded item",
			"- button \"Menu\"",
			"- button \"Menu\" [expanded]\n- menuitem \"Copy\"",
			[]string{`app.Expect(telatest.ByRole("button", "Menu"), telatest.Expanded(true))`, `app.ExpectRole("menuitem", "Copy")`},
		},
		{
			"a quoted text",
			"- paragraph: a",
			"- paragraph: \"Say \\\"hi\\\": now\"",
			[]string{`app.ExpectText("Say \"hi\": now")`},
		},
	} {
		got := expectations(tc.before, tc.after)
		if strings.Join(got, "\n") != strings.Join(tc.want, "\n") {
			t.Errorf("%s:\n got %q\nwant %q", tc.name, got, tc.want)
		}
	}
	// A step that changes the whole screen is capped.
	var after []string
	for i := 0; i < 20; i++ {
		after = append(after, `- button "B`+string(rune('a'+i))+`"`)
	}
	got := expectations("", strings.Join(after, "\n"))
	if len(got) != maxExpectations+1 || !strings.HasPrefix(got[len(got)-1], "// ... and 12 more changes") {
		t.Errorf("the cap: %d lines, last %q", len(got), got[len(got)-1])
	}
	// Two elements with one role and name are told apart by their position.
	got = expectations(`- button "Remove"`+"\n"+`- button "Remove"`, `- button "Remove" [disabled]`+"\n"+`- button "Remove"`)
	if len(got) != 1 || got[0] != `app.Expect(telatest.ByRole("button", "Remove").Nth(0), telatest.Disabled(true))` {
		t.Errorf("an Nth for the first of two: %q", got)
	}
}

func TestStableLocatorsOfRefs(t *testing.T) {
	yaml := "- button \"Save\"\n- button \"Remove\"\n- button \"Remove\"\n- heading\n- link \"Home\""
	lines, cands, _, _ := prepareRefs(yaml)
	tab := newRefTable()
	nodes := []uint64{11, 12, 13, 14, 15}
	tab.assignNodes(lines, cands, nodes)
	for ref, want := range map[string]string{
		"e1": `telatest.ByRole("button", "Save")`,
		"e2": `telatest.ByRole("button", "Remove").Nth(0)`,
		"e3": `telatest.ByRole("button", "Remove").Nth(1)`,
		"e5": `telatest.ByRole("link", "Home")`,
	} {
		e, err := tab.lookup(ref)
		if err != nil {
			t.Fatal(err)
		}
		loc := e.loc
		loc.Stable = &e.stable
		if got := loc.goExpr(); got != want {
			t.Errorf("%s: %s, want %s", ref, got, want)
		}
		if e.loc.NodeID == 0 {
			t.Errorf("%s lost its identity", ref)
		}
	}
	// The same through the headless counts path, which has no identities.
	lines, cands, groups, want := prepareRefs(yaml)
	got := make([]int, len(groups))
	copy(got, want)
	tab = newRefTable()
	tab.assign(lines, cands, want, got)
	e, _ := tab.lookup("e3")
	if g := e.stable.goExpr(); g != `telatest.ByRole("button", "Remove").Nth(1)` {
		t.Errorf("by counts: %s", g)
	}
}

func TestPreferLines(t *testing.T) {
	a := mcpMount{}
	b := mcpMount{Dark: true, Prefer: mcpPrefer{ReducedMotion: true, HighContrast: true, TextScale: 1.5, Locale: "ar", LocaleSet: true}}
	got := strings.Join(preferLines(a, b), "\n")
	for _, want := range []string{"app.SetDarkMode(true)", "app.SetReducedMotion(true)", "app.SetHighContrast(true)", "app.SetTextScale(1.5)", "// the session set the locale to \"ar\""} {
		if !strings.Contains(got, want) {
			t.Errorf("preferLines lacks %q:\n%s", want, got)
		}
	}
	if lines := preferLines(b, b); len(lines) != 0 {
		t.Errorf("no change: %q", lines)
	}
	if got := strings.Join(preferLines(b, a), "\n"); !strings.Contains(got, "app.SetTextScale(1)") || !strings.Contains(got, "app.SetDarkMode(false)") {
		t.Errorf("back to the defaults:\n%s", got)
	}
}

func TestGenerateTestSource(t *testing.T) {
	form := &mcpSegment{
		mount:   mcpMount{W: 480, H: 320, Scale: 1},
		preview: "Form",
		steps: []mcpStep{
			{req: mcpReq{Op: "click", Loc: &mcpLoc{NodeID: 7, Ref: "e1", Stable: &mcpLoc{Role: "button", Name: "Count: 0"}}}, before: `- 'button "Count: 0"'`, after: `- 'button "Count: 1"'`},
			{req: mcpReq{Op: "click", At: &mcpPoint{100, 40}, Double: true}, before: "- heading", after: "- heading"},
			{req: mcpReq{Op: "click", Button: "right", Loc: &mcpLoc{Role: "row", Name: "A"}}},
			{req: mcpReq{Op: "fill", Loc: &mcpLoc{Role: "textbox", Name: "Name"}, Text: "Ana", Submit: true}, before: `- textbox "Name"`, after: `- textbox "Name": Ana`},
			{req: mcpReq{Op: "press", Key: "Escape", Loc: &mcpLoc{TestID: "field"}}},
			{req: mcpReq{Op: "scroll", DY: 300}, w: 480, h: 320},
			{req: mcpReq{Op: "scroll", Loc: &mcpLoc{Role: "list"}, DY: 50}},
			{req: mcpReq{Op: "drag", Loc: &mcpLoc{Role: "slider"}, DX: 40}},
			{req: mcpReq{Op: "drag", At: &mcpPoint{1, 2}, ToAt: &mcpPoint{30, 40}}},
			{req: mcpReq{Op: "wait", Ms: 250, Text: "Done"}},
			{req: mcpReq{Op: "resize", W: 390, H: 844, Scale: 2}},
			{lines: []string{"app.SetDarkMode(true)"}, before: `- button "Go"`, after: `- button "Go"`},
		},
	}
	other := &mcpSegment{
		mount:   mcpMount{W: 300, H: 200, SizeSet: true, Scale: 2, Dark: true, Prefer: mcpPrefer{ReducedMotion: true, TextScale: 1.25, Direction: "rtl"}},
		preview: "Other",
		steps:   []mcpStep{{req: mcpReq{Op: "hover", Loc: &mcpLoc{Text: "Hi", Exact: true}}}},
	}
	for _, tc := range []struct {
		name string
		in   testgenInput
		want []string
		not  []string
	}{
		{
			"previews and an app",
			testgenInput{Pkg: "demo", Func: "TestDemo", Segments: []*mcpSegment{form, {mount: mcpMount{W: 1, H: 1, Scale: 1}}, other}, Contract: contractPreviews, HasApp: true, AppPtr: true},
			[]string{
				"package demo", `"time"`, `"github.com/kuronosu/tela"`, "func TestDemo(t *testing.T) {",
				`app := telatest.Mount(t, Previews["Form"], telatest.Size(480, 320), telatest.FromApp(*App))`,
				`app.Click(telatest.ByRole("button", "Count: 0"))`, `app.ExpectRole("button", "Count: 1")`,
				`app.ClickAt(100, 40)`, "app.MouseDown()", `app.Click(telatest.ByRole("row", "A"), telatest.Right())`,
				`app.Fill(telatest.ByRole("textbox", "Name"), "Ana")`, `app.Press("Enter")`, `app.ExpectValue(`,
				`app.Focus(telatest.ByTestID("field"))`, `app.Press("Escape")`,
				`app.ScrollAt(240, 160, 0, 300)`, `app.Scroll(telatest.ByRole("list", ""), 0, 50)`,
				`app.DragBy(telatest.ByRole("slider", ""), 40, 0)`, `app.DragFromTo(tela.Point{X: 1, Y: 2}, tela.Point{X: 30, Y: 40})`,
				`app.Advance(250 * time.Millisecond)`, `app.ExpectText("Done")`,
				`app.Resize(390, 844)`, `app.SetScale(2)`, `app.SetDarkMode(true)`,
				`app = telatest.Mount(t, Previews["Other"],`, "telatest.Scale(2)", "telatest.Dark()", "telatest.ReducedMotion()", "telatest.TextScale(1.25)",
				`the session mounted the screen with the direction "rtl"`, `app.Hover(telatest.ByText("Hi", telatest.Exact()))`,
				"13 actions", "2 checks", // a segment without steps adds none
			},
			[]string{"NodeID", "getByNodeId", "e1"},
		},
		{
			"a Preview",
			testgenInput{Pkg: "demo", Func: "TestPreview", Segments: []*mcpSegment{{mount: mcpMount{W: 960, H: 640, Scale: 1}, steps: []mcpStep{{req: mcpReq{Op: "hover", Loc: &mcpLoc{Role: "button", Name: "Go"}}}}}}, Contract: contractPreview},
			[]string{`app := telatest.Mount(t, tela.C(Preview), telatest.Size(960, 640))`, `"github.com/kuronosu/tela"`},
			[]string{`"time"`},
		},
		{
			"an App without the tela import",
			testgenInput{Pkg: "main", Func: "TestApp", Segments: []*mcpSegment{{mount: mcpMount{W: 800, H: 600, SizeSet: true, Scale: 1}, steps: []mcpStep{{req: mcpReq{Op: "press", Key: "Tab"}}}}}, Contract: contractApp, HasApp: true},
			[]string{"package main", "app := telatest.Mount(t, App.Root,", "telatest.FromApp(App),\n\t\ttelatest.Size(800, 600),"},
			[]string{`"github.com/kuronosu/tela"`, `"time"`},
		},
	} {
		src, err := generateTestSource(tc.in)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		s := string(src)
		if _, err := parser.ParseFile(token.NewFileSet(), "x_test.go", src, parser.AllErrors); err != nil {
			t.Fatalf("%s: does not parse: %v\n%s", tc.name, err, s)
		}
		if out, err := format.Source(src); err != nil || !bytes.Equal(out, src) {
			t.Errorf("%s: not gofmt'd (%v):\n%s", tc.name, err, s)
		}
		if !strings.HasPrefix(s, testgenMarker) {
			t.Errorf("%s: the file does not begin with the marker the tool replaces its files by", tc.name)
		}
		for _, w := range tc.want {
			if !strings.Contains(s, w) {
				t.Errorf("%s: lacks %q:\n%s", tc.name, w, s)
			}
		}
		for _, n := range tc.not {
			if strings.Contains(s, n) {
				t.Errorf("%s: has %q:\n%s", tc.name, n, s)
			}
		}
	}
	for name, in := range map[string]testgenInput{
		"no steps":        {Pkg: "p", Func: "TestX", Segments: []*mcpSegment{{mount: mcpMount{}}}, Contract: contractPreview},
		"unnamed preview": {Pkg: "p", Func: "TestX", Segments: []*mcpSegment{{steps: []mcpStep{{req: mcpReq{Op: "press", Key: "a"}}}}}, Contract: contractPreviews},
	} {
		if _, err := generateTestSource(in); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestRecorderSegments(t *testing.T) {
	var r mcpRecorder
	step := mcpStep{req: mcpReq{Op: "press", Key: "a"}}
	r.add(step, "ignored") // nothing is mounted yet
	if r.count() != 0 {
		t.Error("a step was recorded before any mount")
	}
	r.reset(mcpMount{W: 1})
	r.add(step, "Form")
	r.add(step, "Other") // the segment keeps the preview it began with
	if r.count() != 2 || r.segs[0].preview != "Form" {
		t.Errorf("%d steps, preview %q", r.count(), r.segs[0].preview)
	}
	r.newSegment(mcpMount{W: 2}, "Other")
	r.newSegment(mcpMount{W: 3}, "Third") // an empty segment is replaced
	if len(r.segs) != 2 || r.segs[1].mount.W != 3 || r.segs[1].preview != "Third" {
		t.Errorf("segments: %+v", r.segs)
	}
	r.reset(mcpMount{})
	if r.count() != 0 || len(r.segs) != 1 {
		t.Errorf("after reset: %d steps in %d segments", r.count(), len(r.segs))
	}
}
