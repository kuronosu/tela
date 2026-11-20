package platform

import (
	"errors"
	"image"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// The checks and the start-at-login flag of contract v16 (ADR-298).

func TestStripAtLoginFlag(t *testing.T) {
	for _, c := range []struct {
		in, want []string
		at       bool
	}{
		{[]string{"notes"}, []string{"notes"}, false},
		{[]string{"notes", "a.md"}, []string{"notes", "a.md"}, false},
		{[]string{"notes", AtLoginFlag}, []string{"notes"}, true},
		{[]string{"notes", "--profile", "x", AtLoginFlag}, []string{"notes", "--profile", "x"}, true},
		{[]string{AtLoginFlag, "a"}, []string{AtLoginFlag, "a"}, false}, // the program's name
		{[]string{"notes", "--", AtLoginFlag}, []string{"notes", "--", AtLoginFlag}, false},
		{[]string{"notes", AtLoginFlag, "--", AtLoginFlag}, []string{"notes", "--", AtLoginFlag}, true},
	} {
		in := slices.Clone(c.in)
		got, at := StripAtLoginFlag(in)
		if !slices.Equal(got, c.want) || at != c.at {
			t.Errorf("StripAtLoginFlag(%q) = %q, %v; want %q, %v", c.in, got, at, c.want, c.at)
		}
		if !slices.Equal(in, c.in) {
			t.Errorf("StripAtLoginFlag(%q) modified its argument", c.in)
		}
	}
}

func TestCheckTasks(t *testing.T) {
	icon := []image.Image{image.NewNRGBA(image.Rect(0, 0, 16, 16))}
	CheckTasks("SetTasks", nil)
	CheckTasks("SetTasks", []Task{{Title: "New note", Args: []string{"notes:new"}, Description: "Two\nlines", Icon: icon}})
	many := make([]Task, MaxTasks+1)
	for i := range many {
		many[i].Title = "x"
	}
	for name, tasks := range map[string][]Task{
		"no Title":          {{Args: []string{"x"}}},
		"blank Title":       {{Title: "  "}},
		"two-line Title":    {{Title: "a\nb"}},
		"control character": {{Title: "a", Description: "\x07"}},
		"NUL argument":      {{Title: "a", Args: []string{"a\x00"}}},
		"the login flag":    {{Title: "a", Args: []string{AtLoginFlag}}},
		"empty icon":        {{Title: "a", Icon: []image.Image{image.NewNRGBA(image.Rectangle{})}}},
		"too many":          many,
	} {
		func() {
			defer func() {
				if p := recover(); p == nil || !strings.Contains(p.(string), "SetTasks") {
					t.Errorf("%s: CheckTasks panicked with %v", name, p)
				}
			}()
			CheckTasks("SetTasks", tasks)
		}()
	}
}

func TestCheckRecent(t *testing.T) {
	for _, p := range []string{"", "a.md", "/a\x00"} {
		if _, err := CheckRecentPath(p); !errors.Is(err, fs.ErrInvalid) {
			t.Errorf("CheckRecentPath(%q) = %v, want fs.ErrInvalid", p, err)
		}
	}
	dir := t.TempDir()
	if _, _, err := CheckRecent(filepath.Join(dir, "missing.md")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("CheckRecent of a missing file = %v", err)
	}
	f := filepath.Join(dir, "a.md")
	if err := os.WriteFile(f, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if got, fi, err := CheckRecent(f + "/."); err != nil || got != f || fi.IsDir() {
		t.Errorf("CheckRecent(%q) = %q, %v", f+"/.", got, err)
	}
}

func TestUnsupportedSurfaces(t *testing.T) {
	var d Desktop = UnsupportedDesktop{}
	app := HandlerApp{ID: "com.example.Notes", Name: "Notes", Exec: "/opt/notes"}
	if err := d.SetStartAtLogin(app, true); !errors.Is(err, ErrUnsupported) {
		t.Errorf("SetStartAtLogin = %v", err)
	}
	if on, err := d.StartsAtLogin(HandlerApp{ID: app.ID}); on || !errors.Is(err, ErrUnsupported) {
		t.Errorf("StartsAtLogin = %v, %v", on, err)
	}
	if err := d.SetTasks(app, []Task{{Title: "New"}}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("SetTasks = %v", err)
	}
	if err := d.AddRecentDocument(app, "a.md"); !errors.Is(err, fs.ErrInvalid) {
		t.Errorf("AddRecentDocument of a relative path = %v, want fs.ErrInvalid", err)
	}
	if err := d.ClearRecentDocuments(HandlerApp{ID: app.ID}); !errors.Is(err, ErrUnsupported) {
		t.Errorf("ClearRecentDocuments = %v", err)
	}
	defer func() {
		if recover() == nil {
			t.Error("SetTasks with an invalid task did not panic where the service is missing")
		}
	}()
	_ = d.SetTasks(app, []Task{{}})
}

func TestCloneTasks(t *testing.T) {
	in := []Task{{Title: "a", Args: []string{"x"}}}
	out := CloneTasks(in)
	out[0].Args[0] = "y"
	if in[0].Args[0] != "x" || CloneTasks(nil) != nil {
		t.Error("CloneTasks shares Args, or makes a slice of nil")
	}
}
