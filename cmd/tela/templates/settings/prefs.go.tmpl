package main

import (
	"regexp"
	"strconv"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/kit"
)

// Prefs is the whole form: comparable, so prefs != saved means "unsaved".
type Prefs struct {
	// General
	Library, Import string
	SkipDuplicates  bool
	Language        string
	// Develop
	Look             string
	Sharpen, Noise   float64
	Lens, Straighten bool
	// Export
	Format, ColorSpace string
	Quality            float64
	Resize, Custom     string
	Watermark          bool
	WmText, WmColor    string
	// Safelight
	Theme, Density string
	Accent         tela.Color
	Thumb          float64
	// Account
	Name, Email string
}

var safelight = tela.Hex("#c0281c")

var defaults = Prefs{
	Library: "/home/ana/Pictures/Darkroom Library", Import: "copy", SkipDuplicates: true, Language: "en",
	Look: "portra", Sharpen: 40, Noise: 25, Lens: true, Straighten: false,
	Format: "jpeg", ColorSpace: "srgb", Quality: 90, Resize: "off", Custom: "3000",
	Watermark: false, WmText: "© Ana Ruiz", WmColor: "white",
	Theme: "system", Density: "default", Accent: safelight, Thumb: 160,
	Name: "Ana Ruiz", Email: "ana.ruiz@darkroom.app",
}

var emailRE = regexp.MustCompile(`^[^@\s]+@[^@\s]+\.[^@\s]{2,}$`)

func emailError(s string) string {
	if !emailRE.MatchString(s) {
		return "That address doesn't look right. Try name@domain.com."
	}
	return ""
}

func customError(p Prefs) string {
	if p.Resize != "custom" {
		return ""
	}
	n, err := strconv.Atoi(p.Custom)
	if err != nil || n < 256 || n > 16384 {
		return "Enter a whole number of pixels between 256 and 16384."
	}
	return ""
}

func (p Prefs) valid() bool { return emailError(p.Email) == "" && customError(p) == "" }

// changed reports whether the fields of a section differ from base's: the
// sidebar marks the sections that have unsaved changes.
func (p Prefs) changed(section string, base Prefs) bool {
	q := base
	switch section {
	case "General":
		q.Library, q.Import, q.SkipDuplicates, q.Language = p.Library, p.Import, p.SkipDuplicates, p.Language
	case "Develop":
		q.Look, q.Sharpen, q.Noise, q.Lens, q.Straighten = p.Look, p.Sharpen, p.Noise, p.Lens, p.Straighten
	case "Export":
		q.Format, q.ColorSpace, q.Quality, q.Resize, q.Custom = p.Format, p.ColorSpace, p.Quality, p.Resize, p.Custom
		q.Watermark, q.WmText, q.WmColor = p.Watermark, p.WmText, p.WmColor
	case "Safelight":
		q.Theme, q.Density, q.Accent, q.Thumb = p.Theme, p.Density, p.Accent, p.Thumb
	case "Account":
		q.Name, q.Email = p.Name, p.Email
	}
	return q != base
}

func scenario(name string) Prefs {
	p := defaults
	switch name {
	case "Unsaved":
		p.Look = "trix"
		p.Sharpen = 72
		p.Noise = 40
		p.Straighten = true
	case "Invalid":
		p.Email = "ana.ruiz@darkroom"
	}
	return p
}

// densityGap is the space between the blocks of a section for each density.
var densityGap = map[string]string{"compact": "gap-4", "default": "gap-6", "comfortable": "gap-8"}

var accents = []struct {
	Key, Label string
	Color      tela.Color
}{
	{"safelight", "Safelight", safelight},
	{"amber", "Amber", tela.Hex("#b7650f")},
	{"sepia", "Sepia", tela.Hex("#8a5a3c")},
	{"cyanotype", "Cyanotype", tela.Hex("#1f6f9c")},
	{"silver", "Silver", tela.Hex("#5b6470")},
}

func accentKey(c tela.Color) string {
	for _, a := range accents {
		if a.Color == c {
			return a.Key
		}
	}
	return ""
}

// makeTheme is the kit theme tinted like a darkroom: warm neutrals, one accent.
func makeTheme(accent tela.Color) tela.Theme {
	t := kit.Theme()
	set := func(k, l, d string) { t.Colors[k] = tela.SchemeColor{Light: tela.Hex(l), Dark: tela.Hex(d)} }
	set("background", "#f4f0ea", "#141110")
	set("foreground", "#1f1a17", "#ece6df")
	set("card", "#fbf9f5", "#1c1816")
	set("card-foreground", "#1f1a17", "#ece6df")
	set("popover", "#fbf9f5", "#201b19")
	set("popover-foreground", "#1f1a17", "#ece6df")
	set("secondary", "#e9e3da", "#2a2421")
	set("secondary-foreground", "#1f1a17", "#ece6df")
	set("muted", "#ebe5dc", "#241f1c")
	set("muted-foreground", "#6f655c", "#a0948a")
	set("accent", "#e6dfd4", "#2e2723")
	set("accent-foreground", "#1f1a17", "#ece6df")
	set("border", "#ddd5c9", "#332c28")
	set("input", "#d3cabc", "#40372f")
	set("destructive", "#b42318", "#f0645a")
	set("primary-foreground", "#ffffff", "#ffffff")
	t.Colors["primary"] = tela.SchemeColor{Light: accent, Dark: accent}
	t.Colors["ring"] = tela.SchemeColor{Light: accent, Dark: accent}
	return t
}
