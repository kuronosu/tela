package kit_test

import (
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

var team = []kit.Avatar{
	{Name: "Mei Tanaka", Color: kit.AvatarAuto},
	{Name: "Ada Lovelace", Color: kit.AvatarAuto},
	{Name: "Alan Turing", Color: kit.AvatarAuto},
	{Name: "Grace Hopper", Color: kit.AvatarAuto},
	{Name: "Linus Torvalds", Color: kit.AvatarAuto},
	{Name: "Margaret Hamilton", Color: kit.AvatarAuto},
}

func avatarGroupSheet() *sheet {
	g := func(name string, grp kit.AvatarGroup) cell {
		grp.Props = h.TestID(name + "-group")
		return cell{name: name, width: 280, probes: []string{name + "-group"}, tela: grp}
	}
	picture := team[0]
	picture.Src, picture.Color = h.Src(portrait()), ""
	return &sheet{name: "avatar_group", width: 640, cells: []cell{
		g("all", kit.AvatarGroup{Avatars: team[:4]}),
		g("max", kit.AvatarGroup{Avatars: team, Max: 3}),
		g("total", kit.AvatarGroup{Avatars: team[:3], Total: 12}),
		g("sm", kit.AvatarGroup{Avatars: team, Max: 4, Size: kit.Sm}),
		g("lg", kit.AvatarGroup{Avatars: team, Max: 4, Size: kit.Lg}),
		g("pictures", kit.AvatarGroup{Avatars: []kit.Avatar{picture, team[1], team[2]}, Max: 3, Total: 5}),
		g("muted", kit.AvatarGroup{Avatars: []kit.Avatar{{Name: "Mei Tanaka"}, {Name: "Ada Lovelace"}, {Name: "Alan Turing"}}, Max: 2}),
		{name: "on-card", width: 280, tela: kit.Card{Class: "p-4", Children: kit.AvatarGroup{Avatars: team, Max: 3, AvatarClass: "ring-card"}}},
	}}
}

func TestAvatarGroupGolden(t *testing.T) { avatarGroupSheet().golden(t) }

func avatarColorSheet() *sheet {
	var cells []cell
	for _, c := range []kit.AvatarColor{"red", "orange", "amber", "emerald", "teal", "sky", "indigo", "violet", "pink"} {
		cells = append(cells, cell{name: string(c), tela: kit.Avatar{Name: "Mei Tanaka", Color: c}})
	}
	cells = append(cells,
		cell{name: "auto-sm", tela: kit.Avatar{Name: "Ada Lovelace", Color: kit.AvatarAuto, Size: kit.Sm}},
		cell{name: "auto", tela: kit.Avatar{Name: "Alan Turing", Color: kit.AvatarAuto}},
		cell{name: "auto-lg", tela: kit.Avatar{Name: "Grace Hopper", Color: kit.AvatarAuto, Size: kit.Lg}},
		cell{name: "fallback", tela: kit.Avatar{Fallback: "CN", Color: kit.AvatarAuto}},
		cell{name: "one-word", tela: kit.Avatar{Name: "Ada", Color: kit.AvatarAuto}},
		cell{name: "muted", tela: kit.Avatar{Name: "Mei Tanaka"}},
	)
	return &sheet{name: "avatar_color", slot: 64, cells: cells}
}

func TestAvatarColorGolden(t *testing.T) { avatarColorSheet().golden(t) }

func TestAvatarGroupRTLGolden(t *testing.T) {
	s := avatarGroupSheet()
	s.name, s.rtl = "avatar_group_rtl", true
	s.cells = s.cells[:3]
	s.golden(t)
}

func TestAvatarInitials(t *testing.T) {
	for name, want := range map[string]string{
		"Mei Tanaka":           "MT",
		"ada lovelace":         "AL",
		"Ada":                  "A",
		"  Grace   Hopper  ":   "GH",
		"Jean-Luc Picard":      "JP",
		"Margaret H. Hamilton": "MH",
		"田中 太郎":                "田太",
		"":                     "",
		"   ":                  "",
	} {
		if got := kit.Initials(name); got != want {
			t.Errorf("Initials(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestAvatarNameIsTheFallbackAndTheAlt(t *testing.T) {
	app := telatest.Mount(t, kit.Avatar{Name: "Mei Tanaka"})
	app.ExpectText("MT")
	// An explicit Fallback wins over the initials.
	app = telatest.Mount(t, kit.Avatar{Name: "Mei Tanaka", Fallback: "me"})
	app.ExpectText("me")
	app.ExpectNoText("MT")
	app = telatest.Mount(t, kit.Avatar{Name: "Mei Tanaka", Src: h.Src(portrait())})
	app.ExpectRole("img", "Mei Tanaka")
	app = telatest.Mount(t, kit.Avatar{Name: "Mei Tanaka", Alt: "Portrait", Src: h.Src(portrait())})
	app.ExpectRole("img", "Portrait")
}

// A name always gets the same color, and the colors spread over the
// palette.
func TestAvatarAutoColorFollowsTheName(t *testing.T) {
	colors := map[string]bool{}
	pick := func(name string) string {
		app := telatest.Mount(t, kit.Avatar{Name: name, Color: kit.AvatarAuto})
		for _, c := range []string{"red", "orange", "amber", "emerald", "teal", "sky", "indigo", "violet", "pink"} {
			if app.Count(telatest.ByClass("bg-"+c+"-600")) == 1 || app.Count(telatest.ByClass("bg-"+c+"-700")) == 1 {
				return c
			}
		}
		t.Fatalf("%q: no palette color", name)
		return ""
	}
	for _, p := range team {
		a, b := pick(p.Name), pick(p.Name)
		if a != b {
			t.Errorf("%q: %s then %s", p.Name, a, b)
		}
		colors[a] = true
	}
	if len(colors) < 3 {
		t.Errorf("six names got %d colors: %v", len(colors), colors)
	}
	// The case and the space around a name do not matter.
	if pick("MEI TANAKA ") != pick("mei tanaka") {
		t.Error("the color depends on the case")
	}
}

func TestAvatarGivenColor(t *testing.T) {
	app := telatest.Mount(t, kit.Avatar{Name: "Mei Tanaka", Color: "indigo"})
	app.ExpectCount(telatest.ByClass("bg-indigo-600", "text-white"), 1)
	app.ExpectCount(telatest.ByClass("bg-muted"), 1) // the recipe's classes stay under it
}

func TestAvatarColorIsChecked(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("a color the recipe does not define did not panic in dev mode")
		}
	}()
	telatest.Mount(t, kit.Avatar{Name: "Mei", Color: "chartreuse"})
}

func TestAvatarColorFromTheRecipe(t *testing.T) {
	s := kit.DefaultStyle()
	s.Avatar.Colors["brand"] = "bg-fuchsia-600 text-white"
	s.Avatar.AutoColors = map[kit.AvatarColor]bool{"brand": true}
	app := telatest.Mount(t, kit.StyleProvider{Style: &s, Children: kit.Avatar{Name: "Mei Tanaka", Color: kit.AvatarAuto}})
	app.ExpectCount(telatest.ByClass("bg-fuchsia-600"), 1)
}

func TestAvatarGroupCountsWhatDoesNotFit(t *testing.T) {
	for name, c := range map[string]struct {
		g    kit.AvatarGroup
		more string
	}{
		"all":    {kit.AvatarGroup{Avatars: team[:3]}, ""},
		"max":    {kit.AvatarGroup{Avatars: team, Max: 4}, "+2"},
		"total":  {kit.AvatarGroup{Avatars: team[:3], Total: 10}, "+7"},
		"both":   {kit.AvatarGroup{Avatars: team, Max: 2, Total: 10}, "+8"},
		"max>n":  {kit.AvatarGroup{Avatars: team[:2], Max: 5}, ""},
		"total<": {kit.AvatarGroup{Avatars: team, Total: 3}, ""},
		"max=n":  {kit.AvatarGroup{Avatars: team, Max: 6}, ""},
	} {
		app := telatest.Mount(t, c.g)
		if c.more == "" {
			app.ExpectNoText("+")
		} else {
			app.ExpectText(c.more)
		}
		shown := len(c.g.Avatars)
		if c.g.Max > 0 && c.g.Max < shown {
			shown = c.g.Max
		}
		app.ExpectCount(telatest.ByClass("ring-2"), shown+map[bool]int{true: 0, false: 1}[c.more == ""])
		_ = name
	}
}

func TestAvatarGroupIsAGroup(t *testing.T) {
	app := telatest.Mount(t, kit.AvatarGroup{AriaLabel: "Team", Avatars: team[:2], Max: 1})
	app.ExpectCount(telatest.ByRole("group", "Team"), 1)
}

// The avatars overlap by 8 px, the next one over the previous.
func TestAvatarGroupOverlaps(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Class("w-80"), kit.AvatarGroup{Avatars: []kit.Avatar{
			{Name: "Ada Lovelace", Props: h.TestID("a")}, {Name: "Alan Turing", Props: h.TestID("b")}}})
	})
	a, b := app.Bounds(telatest.ByTestID("a")), app.Bounds(telatest.ByTestID("b"))
	if got := a.X + a.Width - b.X; got != 8 {
		t.Errorf("the avatars overlap by %v px, want 8", got)
	}
}

// Under a right-to-left direction the overlap mirrors: the next avatar is
// at the left.
func TestAvatarGroupMirrors(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return h.Div(h.Dir("rtl"), h.Class("w-80"), kit.AvatarGroup{Avatars: []kit.Avatar{
			{Name: "Ada Lovelace", Props: h.TestID("a")}, {Name: "Alan Turing", Props: h.TestID("b")}}})
	})
	a, b := app.Bounds(telatest.ByTestID("a")), app.Bounds(telatest.ByTestID("b"))
	if b.X >= a.X || b.X+b.Width-a.X != 8 {
		t.Errorf("rtl: first at %v, second at %v (width %v)", a.X, b.X, b.Width)
	}
}

func TestAvatarGroupSizeAppliesToTheOnesWithout(t *testing.T) {
	app := telatest.MountFunc(t, func() tela.Node {
		return kit.AvatarGroup{Size: kit.Lg, Max: 2, Avatars: []kit.Avatar{
			{Name: "Ada Lovelace", Props: h.TestID("a")}, {Name: "Alan Turing", Size: kit.Sm, Props: h.TestID("b")}, {Name: "Grace Hopper"}}}
	})
	if w := app.Bounds(telatest.ByTestID("a")).Width; w != 40 {
		t.Errorf("the group's size gave %v px, want 40", w)
	}
	if w := app.Bounds(telatest.ByTestID("b")).Width; w != 24 {
		t.Errorf("an avatar's own size gave %v px, want 24", w)
	}
}
