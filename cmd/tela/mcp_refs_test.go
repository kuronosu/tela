package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// refsFor gives refs to a snapshot whose counts all match.
func refsFor(tab *refTable, yaml string) string {
	lines, cands, _, want := prepareRefs(yaml)
	return tab.assign(lines, cands, want, slices.Clone(want))
}

func TestRefsAreStable(t *testing.T) {
	tab := newRefTable()
	first := refsFor(tab, `- heading "Settings" [level=1]
- 'button "Count: 0"'
- button "Save"
- button "Save"
- text: Saved`)
	want := `- heading "Settings" [level=1] [ref=e1]
- 'button "Count: 0" [ref=e2]'
- button "Save" [ref=e3]
- button "Save" [ref=e4]
- text: Saved`
	if first != want {
		t.Fatalf("refs:\n%s\nwant\n%s", first, want)
	}
	// Locators: unique names need no Nth, repeated ones do.
	if e, err := tab.lookup("e1"); err != nil || e.loc.Role != "heading" || e.loc.Name != "Settings" || e.loc.Nth != nil {
		t.Errorf("e1 = %+v, %v", e, err)
	}
	if e, _ := tab.lookup("e4"); e.loc.Nth == nil || *e.loc.Nth != 1 || e.loc.Name != "Save" {
		t.Errorf("e4 = %+v, want the second Save", e.loc)
	}

	// A counter whose label changed keeps its ref by position; a new
	// element gets a new number; one that left is stale.
	second := refsFor(tab, `- heading "Settings" [level=1]
- 'button "Count: 1"'
- button "Save"
- link "Help"`)
	want = `- heading "Settings" [level=1] [ref=e1]
- 'button "Count: 1" [ref=e2]'
- button "Save" [ref=e3]
- link "Help" [ref=e5]`
	if second != want {
		t.Errorf("refs after a change:\n%s\nwant\n%s", second, want)
	}
	if _, err := tab.lookup("e4"); err == nil || !strings.Contains(err.Error(), "not in the latest snapshot") {
		t.Errorf("stale ref: %v", err)
	}
	if _, err := tab.lookup("e77"); err == nil || !strings.Contains(err.Error(), "unknown ref") {
		t.Errorf("unknown ref: %v", err)
	}

	// After a remount (a rebuild), the refs must be taken again; the same
	// role and name keep their ref, positions do not carry over, and
	// numbering goes on.
	tab.remount()
	if !tab.stale {
		t.Error("a remount does not mark the refs stale")
	}
	if _, err := tab.lookup("e1"); err == nil {
		t.Error("a ref resolves after a remount, before a new snapshot")
	}
	got := refsFor(tab, `- heading "Settings" [level=1]
- button "Renamed"
- button "Save"`)
	want = `- heading "Settings" [level=1] [ref=e1]
- button "Renamed" [ref=e6]
- button "Save" [ref=e3]`
	if got != want || tab.stale {
		t.Errorf("after a remount (stale %v):\n%s\nwant\n%s", tab.stale, got, want)
	}
	if _, err := tab.lookup("e2"); err == nil || !strings.Contains(err.Error(), "not in the latest snapshot") {
		t.Errorf("a ref of an element that is gone: %v", err)
	}
}

// TestRefsForgetOldNames checks that a ref that followed its element to a
// new name no longer answers to the old one: neither a new element with the
// old name in the same mount nor the old name after a remount takes it.
func TestRefsForgetOldNames(t *testing.T) {
	tab := newRefTable()
	refsFor(tab, `- button "Save"`)  // e1
	refsFor(tab, `- button "Saved"`) // e1, by position
	got := refsFor(tab, `- button "Save"
- button "Saved"`)
	want := `- button "Save" [ref=e2]
- button "Saved" [ref=e1]`
	if got != want {
		t.Errorf("a new element with the old name:\n%s\nwant\n%s", got, want)
	}

	tab = newRefTable()
	refsFor(tab, `- 'button "Count: 0"'`) // e1
	refsFor(tab, `- 'button "Count: 1"'`) // e1, by position
	tab.remount()
	got = refsFor(tab, `- 'button "Count: 0"'`)
	if want := `- 'button "Count: 0" [ref=e2]'`; got != want {
		t.Errorf("the old name after a remount:\n%s\nwant\n%s", got, want)
	}
}

func TestRefsNeedMatchingCounts(t *testing.T) {
	tab := newRefTable()
	lines, cands, groups, want := prepareRefs(`- button "A"
- generic "x"
- generic "x"
- button`)
	if len(groups) != 3 {
		t.Fatalf("groups %v, want button A, generic x and button (no name)", groups)
	}
	got := slices.Clone(want)
	for i, g := range groups {
		if g.Role == "generic" {
			got[i] = 3 // an empty explicit generic the snapshot leaves out
		}
	}
	out := tab.assign(lines, cands, want, got)
	if out != `- button "A" [ref=e1]
- generic "x"
- generic "x"
- button [ref=e2]` {
		t.Errorf("refs:\n%s", out)
	}
	e, err := tab.lookup("e2")
	if err != nil || e.loc.Role != "button" || e.loc.Name != "" || e.loc.Nth == nil || *e.loc.Nth != 1 {
		t.Errorf("an unnamed button is ByRole(button).Nth(1) among all buttons: %+v, %v", e.loc, err)
	}
}

func TestLayoutSubtree(t *testing.T) {
	js := []byte(`{"kind":"root","rect":{"x":0,"y":0,"width":100,"height":100},"children":[
	 {"kind":"div","rect":{"x":10,"y":10,"width":50,"height":20},"children":[
	   {"kind":"button","rect":{"x":10,"y":10,"width":50,"height":20},"text":"Go"}]},
	 {"kind":"div","rect":{"x":0,"y":50,"width":100,"height":50},"bounds":{"x":5,"y":60,"width":40,"height":10}}]}`)
	sub, err := layoutSubtree(js, [4]float64{10, 10, 50, 20}, "button")
	if err != nil || !strings.HasPrefix(string(sub), "{\n  \"kind\": \"button\"") {
		t.Errorf("button: %s, %v", sub, err)
	}
	sub, err = layoutSubtree(js, [4]float64{10, 10, 50, 20}, "group")
	if err != nil || !strings.HasPrefix(string(sub), "{\n  \"kind\": \"div\"") || !strings.Contains(string(sub), `"button"`) {
		t.Errorf("an element without an implicit role is the outermost box: %s, %v", sub, err)
	}
	sub, err = layoutSubtree(js, [4]float64{5, 60, 40, 10}, "img")
	if err != nil || !strings.Contains(string(sub), `"bounds"`) {
		t.Errorf("a transformed box is found by its bounds: %s, %v", sub, err)
	}
	sub, err = layoutSubtree(js, [4]float64{12, 12, 5, 5}, "link")
	if err != nil || !strings.Contains(string(sub), `"button"`) {
		t.Errorf("without an exact box, the innermost that contains it: %s, %v", sub, err)
	}
}

func TestScanPackagePreviews(t *testing.T) {
	dir := t.TempDir()
	src := `package p

import "github.com/kuronosu/tela"

var Previews = tela.Previews{
	"Settings": {Root: tela.C(settings)},
	"Login":    {Root: Login{}},
}
`
	if err := os.WriteFile(filepath.Join(dir, "p.go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	det, err := scanPackage(dir, []string{"p.go"})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(det.previews, []string{"Login", "Settings"}) {
		t.Errorf("names %v", det.previews)
	}
}

func TestTrimPanic(t *testing.T) {
	p := "boom\n\ngoroutine 8 [running]:\nruntime/debug.Stack()\n\t/x/stack.go:26\napp.telaMCPProtect.func1()\n\t/x/zz.go:1\npanic({0x1, 0x2})\n\t/x/panic.go:859\napp.form.func4()\n\t/x/app.go:34 +0x25\n"
	if got, want := trimPanic(p), "boom\n\napp.form.func4()\n\t/x/app.go:34 +0x25"; got != want {
		t.Errorf("trimPanic:\n%q\nwant\n%q", got, want)
	}
}
