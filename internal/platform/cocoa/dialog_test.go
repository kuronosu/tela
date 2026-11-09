package cocoa

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/platform"
)

func TestPanelFilters(t *testing.T) {
	got := panelFilters([]platform.FileFilter{
		{Name: "Images", Patterns: []string{"*.png", "*.JPG", "*.png", "*.jpeg"}},
		{Patterns: []string{"README*", "*.md"}},
		{Name: "Archives", Patterns: []string{"*.tar.gz", "*.zip"}},
		{Name: "All files", Patterns: []string{"*"}},
		{Name: "Any", Patterns: []string{"*.*", "*.txt"}},
	})
	want := []string{
		`"Images" exts [png jpg jpeg] globs [] all false`,
		`"README*, *.md" exts [md] globs [readme*] all false`,
		`"Archives" exts [zip] globs [*.tar.gz] all false`,
		`"All files" exts [] globs [] all true`,
		`"Any" exts [txt] globs [] all true`,
	}
	for i, f := range got {
		s := fmt.Sprintf("%q exts %v globs %v all %v", f.label, f.exts, f.globs, f.all)
		if s != want[i] {
			t.Errorf("filter %d = %s, want %s", i, s, want[i])
		}
	}

	// What each panel filters by.
	cases := []struct {
		f        panelFilter
		open     []string
		delegate bool
		save     []string
	}{
		{got[0], []string{"png", "jpg", "jpeg"}, false, []string{"png", "jpg", "jpeg"}},
		{got[1], nil, true, []string{"md"}},
		{got[2], nil, true, []string{"zip"}},
		{got[3], nil, false, nil},
		{got[4], nil, false, nil},
	}
	for i, c := range cases {
		open, del := c.f.openTypes()
		if !slices.Equal(open, c.open) || del != c.delegate {
			t.Errorf("filter %d openTypes = %v, %v; want %v, %v", i, open, del, c.open, c.delegate)
		}
		if save := c.f.saveTypes(); !slices.Equal(save, c.save) {
			t.Errorf("filter %d saveTypes = %v, want %v", i, save, c.save)
		}
	}

	// The delegate's matching, without regard to case.
	for _, m := range []struct {
		f    panelFilter
		name string
		want bool
	}{
		{got[1], "README", true},
		{got[1], "readme.txt", true},
		{got[1], "Notes.MD", true},
		{got[1], ".md", false},
		{got[1], "notes.txt", false},
		{got[2], "backup.TAR.GZ", true},
		{got[2], "backup.gz", false},
		{got[2], "a.zip", true},
		{got[3], "anything", true},
	} {
		if g := m.f.match(m.name); g != m.want {
			t.Errorf("%q match(%q) = %v, want %v", m.f.label, m.name, g, m.want)
		}
	}
}

func TestSimpleExtension(t *testing.T) {
	for _, c := range []struct {
		p, ext string
		ok     bool
	}{
		{"*.png", "png", true},
		{"*.", "", false},
		{"*.tar.gz", "", false},
		{"*.p?g", "", false},
		{"*.[ab]", "", false},
		{"photo.png", "", false},
		{"*png", "", false},
		{"*", "", false},
	} {
		ext, ok := simpleExtension(c.p)
		if ext != c.ext || ok != c.ok {
			t.Errorf("simpleExtension(%q) = %q, %v; want %q, %v", c.p, ext, ok, c.ext, c.ok)
		}
	}
}

func TestAlertMapping(t *testing.T) {
	for _, c := range []struct {
		b      platform.MessageButtons
		titles string
	}{
		{platform.ButtonsOK, "OK"},
		{platform.ButtonsOKCancel, "OK, Cancel(esc)"},
		{platform.ButtonsYesNo, "Yes, No(esc)"},
		{platform.ButtonsYesNoCancel, "Yes, Cancel(esc), No"},
	} {
		var titles []string
		for i, bt := range alertButtons(c.b, nil) {
			s := bt.title
			if bt.escape {
				s += "(esc)"
			}
			titles = append(titles, s)
			if got := alertAnswer(c.b, alertButtonResponse(i)); got != bt.answer {
				t.Errorf("%d: response %d = %v, want %v", c.b, alertFirstButtonReturn+i, got, bt.answer)
			}
			if !c.b.Has(bt.answer) {
				t.Errorf("%d: button %q answers %v, not in the set", c.b, bt.title, bt.answer)
			}
		}
		if got := strings.Join(titles, ", "); got != c.titles {
			t.Errorf("buttons of %d = %s, want %s", c.b, got, c.titles)
		}
		// Anything but a button: the set's dismiss answer.
		for _, resp := range []int{0, 1, -1000, -1001, alertFirstButtonReturn + 3} {
			if got := alertAnswer(c.b, resp); got != c.b.DismissAnswer() {
				t.Errorf("%d: response %d = %v, want the dismiss answer %v", c.b, resp, got, c.b.DismissAnswer())
			}
		}
	}

	styles := map[platform.MessageKind]int{
		platform.MessageInfo:     alertStyleInformational,
		platform.MessageWarning:  alertStyleWarning,
		platform.MessageError:    alertStyleCritical,
		platform.MessageQuestion: alertStyleInformational,
	}
	for k, want := range styles {
		if got := alertStyle(k); got != want {
			t.Errorf("alertStyle(%d) = %d, want %d", k, got, want)
		}
	}

	if m, i := alertText("Save?", "Your changes will be lost."); m != "Save?" || i != "Your changes will be lost." {
		t.Errorf("alertText with a title = %q, %q", m, i)
	}
	if m, i := alertText("", "Done."); m != "Done." || i != "" {
		t.Errorf("alertText without a title = %q, %q; want the text as the message", m, i)
	}
}
