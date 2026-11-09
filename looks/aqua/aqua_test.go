package aqua_test

import (
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/looks"
	"github.com/kuronosu/tela/looks/aqua"
)

// tokens every look defines (package looks), and the ones Aqua's recipes use.
var tokens = []string{
	"background", "foreground", "card", "card-foreground", "popover", "popover-foreground",
	"primary", "primary-foreground", "secondary", "secondary-foreground", "muted", "muted-foreground",
	"accent", "accent-foreground", "destructive", "border", "input", "ring",
	"toolbar", "sidebar", "field", "edge", "selection", "selection-foreground",
	"glass", "glass-strong", "glass-selected", "glass-edge", "glass-highlight", "glass-shadow", "primary-glow",
	"control-shadow", "segment", "destructive-text",
}

var designs = []aqua.Design{aqua.Tahoe, aqua.Sequoia}

// combinations is every look the options can make: the design, the design of
// the controls and of the chrome, and glass content or not.
func combinations() map[string]looks.Look {
	out := map[string]looks.Look{}
	for _, d := range designs {
		for _, c := range designs {
			for _, ch := range designs {
				for _, glass := range []bool{false, true} {
					opts := []aqua.Option{d, aqua.Controls(c), aqua.Chrome(ch)}
					key := fmt.Sprintf("%v/controls %v/chrome %v", d, c, ch)
					if glass {
						opts = append(opts, aqua.GlassContent())
						key += "/glass"
					}
					out[key] = aqua.Look(opts...)
				}
			}
		}
	}
	return out
}

func TestEveryCombinationIsComplete(t *testing.T) {
	for key, l := range combinations() {
		for _, tok := range tokens {
			c, ok := l.Theme.Colors[tok]
			if !ok {
				t.Errorf("%s: token %q is not defined", key, tok)
				continue
			}
			if c.Light == (tela.Color{}) && c.Dark == (tela.Color{}) {
				t.Errorf("%s: token %q is transparent black in both schemes", key, tok)
			}
		}
		if l.Theme.FontSans == "" || l.Theme.Radius <= 0 {
			t.Errorf("%s: no font or radius in the theme", key)
		}
		r := l.Regions
		for field, v := range map[string]string{
			"Window": r.Window, "Toolbar": r.Toolbar, "ToolbarGroup": r.ToolbarGroup, "ToolbarSegments": r.ToolbarSegments,
			"Location": r.Location, "Search": r.Search, "SearchIcon": r.SearchIcon, "Sidebar": r.Sidebar,
			"Content": r.Content, "Details": r.Details, "DetailsTitle": r.DetailsTitle, "DetailsLabel": r.DetailsLabel,
			"StatusBar": r.StatusBar,
		} {
			if v == "" {
				t.Errorf("%s: Regions.%s is empty", key, field)
			}
		}
		g := l.Style.Toggle
		for _, v := range []kit.Variant{looks.Sidebar, looks.Toolbar} {
			if g.Variants[v] == "" {
				t.Errorf("%s: no Toggle variant %q", key, v)
			}
		}
		for _, s := range []kit.Size{looks.SidebarSize, looks.ToolbarSize} {
			if _, ok := g.Sizes[s]; !ok {
				t.Errorf("%s: no Toggle size %q", key, s)
			}
		}
		if !strings.HasPrefix(l.Name, "macOS ") {
			t.Errorf("%s: name %q does not start with macOS", key, l.Name)
		}
	}
}

func TestDefaultIsTahoe(t *testing.T) {
	a, b := aqua.Look(), aqua.Look(aqua.Tahoe)
	if !reflect.DeepEqual(a, b) {
		t.Error("Look() is not Look(Tahoe)")
	}
	if a.Name != "macOS Tahoe" || aqua.Look(aqua.Sequoia).Name != "macOS Sequoia" {
		t.Errorf("names %q and %q", a.Name, aqua.Look(aqua.Sequoia).Name)
	}
}

func TestDesignsDiffer(t *testing.T) {
	a, b := aqua.Look(aqua.Tahoe), aqua.Look(aqua.Sequoia)
	if reflect.DeepEqual(a.Theme, b.Theme) {
		t.Error("the designs have the same theme")
	}
	if reflect.DeepEqual(*a.Style, *b.Style) {
		t.Error("the designs have the same style")
	}
	if a.Regions == b.Regions {
		t.Error("the designs have the same regions")
	}
	if a.Theme.Radius <= b.Theme.Radius {
		t.Errorf("Tahoe's radius %g is not larger than Sequoia's %g", a.Theme.Radius, b.Theme.Radius)
	}
}

// TestOptionsApplyInOrder: the last option that sets a part wins, and a design
// sets every part.
func TestOptionsApplyInOrder(t *testing.T) {
	tahoe, sequoia := aqua.Look(aqua.Tahoe), aqua.Look(aqua.Sequoia)

	// Sequoia with Tahoe's chrome: Sequoia's tokens and controls, Tahoe's regions.
	mix := aqua.Look(aqua.Sequoia, aqua.Chrome(aqua.Tahoe))
	if mix.Regions != tahoe.Regions {
		t.Error("Look(Sequoia, Chrome(Tahoe)) has not Tahoe's regions")
	}
	if !reflect.DeepEqual(mix.Theme, sequoia.Theme) {
		t.Error("Look(Sequoia, Chrome(Tahoe)) has not Sequoia's tokens")
	}
	if mix.Style.Button.Root != sequoia.Style.Button.Root || mix.Style.Checkbox.Root != sequoia.Style.Checkbox.Root {
		t.Error("Look(Sequoia, Chrome(Tahoe)) has not Sequoia's controls")
	}
	if mix.Style.Toggle.Variants[looks.Sidebar] != tahoe.Style.Toggle.Variants[looks.Sidebar] {
		t.Error("Look(Sequoia, Chrome(Tahoe)) has not Tahoe's sidebar entries")
	}
	if want := "macOS Sequoia (mixed: Tahoe chrome)"; mix.Name != want {
		t.Errorf("name %q, want %q", mix.Name, want)
	}

	// The same options the other way round: the design comes last, so it wins.
	back := aqua.Look(aqua.Chrome(aqua.Tahoe), aqua.Sequoia)
	if !reflect.DeepEqual(back, sequoia) {
		t.Error("Look(Chrome(Tahoe), Sequoia) is not Look(Sequoia)")
	}
	if !reflect.DeepEqual(aqua.Look(aqua.Controls(aqua.Sequoia), aqua.Chrome(aqua.Sequoia), aqua.Tahoe), tahoe) {
		t.Error("a design after Controls and Chrome does not reset them")
	}

	// Controls and Chrome of the same design are the design's parts.
	if got := aqua.Look(aqua.Tahoe, aqua.Controls(aqua.Tahoe), aqua.Chrome(aqua.Tahoe)); !reflect.DeepEqual(got, tahoe) {
		t.Error("Tahoe with Tahoe's parts is not Tahoe")
	}

	// Controls alone.
	c := aqua.Look(aqua.Tahoe, aqua.Controls(aqua.Sequoia))
	if c.Regions != tahoe.Regions || !reflect.DeepEqual(c.Theme, tahoe.Theme) {
		t.Error("Controls changed the regions or the tokens")
	}
	if c.Style.Button.Root != sequoia.Style.Button.Root || c.Style.Card.Root != tahoe.Style.Card.Root {
		t.Error("Controls did not set the buttons only")
	}
	if want := "macOS Tahoe (mixed: Sequoia controls)"; c.Name != want {
		t.Errorf("name %q, want %q", c.Name, want)
	}

	// The last of two options for the same part wins.
	if got := aqua.Look(aqua.Controls(aqua.Sequoia), aqua.Controls(aqua.Tahoe)); got.Style.Button.Root != tahoe.Style.Button.Root {
		t.Error("the last Controls does not win")
	}
	if got := aqua.Look(aqua.Controls(aqua.Sequoia), aqua.Controls(aqua.Tahoe)); got.Name != "macOS Tahoe" {
		t.Errorf("a mix undone is named %q", got.Name)
	}
}

func TestChromeRegionsAreTheDesigns(t *testing.T) {
	tahoe, sequoia := aqua.Look(aqua.Tahoe), aqua.Look(aqua.Sequoia)
	if got := aqua.Look(aqua.Tahoe, aqua.Chrome(aqua.Sequoia)).Regions; got != sequoia.Regions {
		t.Error("Chrome(Sequoia) has not Sequoia's regions")
	}
	if got := aqua.Look(aqua.Sequoia, aqua.Chrome(aqua.Tahoe)).Regions; got != tahoe.Regions {
		t.Error("Chrome(Tahoe) has not Tahoe's regions")
	}
}

func TestGlassContent(t *testing.T) {
	plain := aqua.Look(aqua.Tahoe)
	glass := aqua.Look(aqua.Tahoe, aqua.GlassContent())
	if plain.Style.Card.Root == glass.Style.Card.Root {
		t.Error("GlassContent does not change the cards")
	}
	if !strings.Contains(glass.Style.Card.Root, "backdrop-blur") || strings.Contains(plain.Style.Card.Root, "backdrop-blur") {
		t.Errorf("only the glass cards blur: %q and %q", plain.Style.Card.Root, glass.Style.Card.Root)
	}
	// Only the cards change.
	a, b := *plain.Style, *glass.Style
	a.Card, b.Card = kit.CardStyle{}, kit.CardStyle{}
	if !reflect.DeepEqual(a, b) {
		t.Error("GlassContent changed more than the cards")
	}
	if plain.Regions != glass.Regions || !reflect.DeepEqual(plain.Theme, glass.Theme) {
		t.Error("GlassContent changed the regions or the tokens")
	}
	if want := "macOS Tahoe (mixed: glass content)"; glass.Name != want {
		t.Errorf("name %q, want %q", glass.Name, want)
	}
	// Setting the design afterwards makes the content opaque again; before, no.
	if got := aqua.Look(aqua.GlassContent(), aqua.Tahoe); !reflect.DeepEqual(got, plain) {
		t.Error("a design after GlassContent keeps the glass")
	}
	if got := aqua.Look(aqua.Sequoia, aqua.GlassContent()); !strings.Contains(got.Style.Card.Root, "backdrop-blur") {
		t.Error("Sequoia with GlassContent has no glass cards")
	}
	if got, want := aqua.Look(aqua.Sequoia, aqua.Controls(aqua.Tahoe), aqua.GlassContent()).Name, "macOS Sequoia (mixed: Tahoe controls, glass content)"; got != want {
		t.Errorf("name %q, want %q", got, want)
	}
}

func TestSequoiaHasNoBackdropBlur(t *testing.T) {
	l := aqua.Look(aqua.Sequoia)
	var found []string
	var walk func(v reflect.Value, path string)
	walk = func(v reflect.Value, path string) {
		switch v.Kind() {
		case reflect.String:
			if strings.Contains(v.String(), "backdrop-") {
				found = append(found, path)
			}
		case reflect.Struct:
			for i := range v.NumField() {
				walk(v.Field(i), path+"."+v.Type().Field(i).Name)
			}
		case reflect.Map:
			for it := v.MapRange(); it.Next(); {
				walk(it.Value(), fmt.Sprintf("%s[%v]", path, it.Key()))
			}
		}
	}
	walk(reflect.ValueOf(*l.Style), "Style")
	walk(reflect.ValueOf(l.Regions), "Regions")
	if len(found) > 0 {
		t.Errorf("Sequoia blurs the backdrop in %v", found)
	}
}

func TestLookIsNew(t *testing.T) {
	a, b := aqua.Look(), aqua.Look()
	a.Style.Button.Root = "edited"
	a.Theme.Colors["primary"] = tela.SchemeColor{}
	if b.Style.Button.Root == "edited" || b.Theme.Colors["primary"] == (tela.SchemeColor{}) {
		t.Error("two calls share their style or theme")
	}
}

func TestNames(t *testing.T) {
	for _, c := range []struct {
		opts []aqua.Option
		want string
	}{
		{nil, "macOS Tahoe"},
		{[]aqua.Option{aqua.Sequoia}, "macOS Sequoia"},
		{[]aqua.Option{aqua.Tahoe, aqua.Controls(aqua.Sequoia)}, "macOS Tahoe (mixed: Sequoia controls)"},
		{[]aqua.Option{aqua.Tahoe, aqua.Chrome(aqua.Sequoia)}, "macOS Tahoe (mixed: Sequoia chrome)"},
		{[]aqua.Option{aqua.Tahoe, aqua.Controls(aqua.Sequoia), aqua.Chrome(aqua.Sequoia)}, "macOS Tahoe (mixed: Sequoia controls and chrome)"},
		{[]aqua.Option{aqua.Sequoia, aqua.Controls(aqua.Tahoe)}, "macOS Sequoia (mixed: Tahoe controls)"},
		{[]aqua.Option{aqua.Sequoia, aqua.Chrome(aqua.Tahoe)}, "macOS Sequoia (mixed: Tahoe chrome)"},
		{[]aqua.Option{aqua.Sequoia, aqua.GlassContent()}, "macOS Sequoia (mixed: glass content)"},
	} {
		if got := aqua.Look(c.opts...).Name; got != c.want {
			t.Errorf("name %q, want %q", got, c.want)
		}
	}
	names := map[string]string{}
	for key, l := range combinations() {
		if other, ok := names[l.Name]; ok {
			t.Errorf("%s and %s are both named %q", key, other, l.Name)
		}
		names[l.Name] = key
	}
}

func TestInvalidDesignPanicsWithAdvice(t *testing.T) {
	for name, f := range map[string]func(){
		"Look":     func() { aqua.Look(aqua.Design(7)) },
		"Controls": func() { aqua.Controls(aqua.Design(-1)) },
		"Chrome":   func() { aqua.Chrome(aqua.Design(2)) },
		"nil":      func() { aqua.Look(nil) },
	} {
		func() {
			defer func() {
				msg, _ := recover().(string)
				if !strings.Contains(msg, "aqua:") {
					t.Errorf("%s: panic %q does not start with aqua:", name, msg)
				}
				if name != "nil" && !strings.Contains(msg, "aqua.Tahoe or aqua.Sequoia") {
					t.Errorf("%s: panic %q does not say what to use", name, msg)
				}
			}()
			f()
		}()
	}
}

func TestDesignString(t *testing.T) {
	if aqua.Tahoe.String() != "Tahoe" || aqua.Sequoia.String() != "Sequoia" {
		t.Errorf("%v %v", aqua.Tahoe, aqua.Sequoia)
	}
}
