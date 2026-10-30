package looks_test

import (
	"reflect"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/adwaita"
	"github.com/kuronosu/tela/looks/breeze"
	"github.com/kuronosu/tela/looks/win11"
)

var all = map[string]func() looks.Look{"win11": win11.Look, "adwaita": adwaita.Look, "breeze": breeze.Look}

// tokens every look defines: kit's, and the ones of package looks.
var tokens = []string{
	"background", "foreground", "card", "card-foreground", "popover", "popover-foreground",
	"primary", "primary-foreground", "secondary", "secondary-foreground", "muted", "muted-foreground",
	"accent", "accent-foreground", "destructive", "border", "input", "ring",
	"toolbar", "sidebar", "field", "edge", "selection", "selection-foreground",
}

func TestEveryLookDefinesTheTokens(t *testing.T) {
	for name, f := range all {
		l := f()
		for _, tok := range tokens {
			c, ok := l.Theme.Colors[tok]
			if !ok {
				t.Errorf("%s: token %q is not defined", name, tok)
				continue
			}
			if c.Light == (tela.Color{}) && c.Dark == (tela.Color{}) {
				t.Errorf("%s: token %q is transparent black in both schemes", name, tok)
			}
		}
		if l.Theme.FontSans == "" || l.Theme.Radius <= 0 {
			t.Errorf("%s: no font or radius in the theme", name)
		}
	}
}

func TestRegionsHaveWhatEveryLookNeeds(t *testing.T) {
	for name, f := range all {
		r := f().Regions
		for field, v := range map[string]string{
			"Window": r.Window, "Toolbar": r.Toolbar, "ToolbarGroup": r.ToolbarGroup, "Location": r.Location,
			"Search": r.Search, "SearchIcon": r.SearchIcon, "Sidebar": r.Sidebar, "Content": r.Content,
			"Details": r.Details, "DetailsTitle": r.DetailsTitle, "DetailsLabel": r.DetailsLabel, "StatusBar": r.StatusBar,
		} {
			if v == "" {
				t.Errorf("%s: Regions.%s is empty", name, field)
			}
		}
	}
}

func TestEveryLookDefinesTheToggleVariants(t *testing.T) {
	for name, f := range all {
		g := f().Style.Toggle
		for _, v := range []kit.Variant{looks.Sidebar, looks.Toolbar} {
			if _, ok := g.Variants[v]; !ok {
				t.Errorf("%s: no Toggle variant %q", name, v)
			}
		}
		for _, s := range []kit.Size{looks.SidebarSize, looks.ToolbarSize} {
			if _, ok := g.Sizes[s]; !ok {
				t.Errorf("%s: no Toggle size %q", name, s)
			}
		}
	}
}

// TestLooksDiffer: the three looks are three different sets of tokens,
// recipes and regions.
func TestLooksDiffer(t *testing.T) {
	names := []string{"win11", "adwaita", "breeze"}
	for i, a := range names {
		for _, b := range names[i+1:] {
			la, lb := all[a](), all[b]()
			if la.Name == lb.Name {
				t.Errorf("%s and %s have the same Name %q", a, b, la.Name)
			}
			if reflect.DeepEqual(la.Theme, lb.Theme) {
				t.Errorf("%s and %s have the same theme", a, b)
			}
			if reflect.DeepEqual(*la.Style, *lb.Style) {
				t.Errorf("%s and %s have the same style", a, b)
			}
			if la.Regions == lb.Regions {
				t.Errorf("%s and %s have the same regions", a, b)
			}
		}
	}
}

// TestLookIsNew: each call returns a look that can be edited without touching
// the next one.
func TestLookIsNew(t *testing.T) {
	for name, f := range all {
		a, b := f(), f()
		a.Style.Button.Root = "edited"
		a.Theme.Colors["primary"] = tela.SchemeColor{}
		if b.Style.Button.Root == "edited" || b.Theme.Colors["primary"] == (tela.SchemeColor{}) {
			t.Errorf("%s: two calls share their style or theme", name)
		}
	}
}
