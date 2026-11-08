package text

import (
	"runtime"
	"slices"
)

// The generic font families of each operating system (ADR-244): what a
// generic name the collection does not map resolves to when the system
// fonts are on. A name resolves to the first family of its list that is
// installed (Collection.resolveSlots), so the lists go from the system's
// own design to common fallbacks. Browsers do the same with their own
// tables; these follow them where the platform has a designated face and
// fontconfig's defaults elsewhere.

// darwinGenerics: San Francisco is the system face; its files
// (/System/Library/Fonts/SFNS.ttf, SFNSMono.ttf, SFNSRounded.ttf,
// NewYork.ttf) name their families with a leading dot (".SF NS", which
// fontscan reports as ".sfns") and some releases call it "System Font";
// the public names come after for systems that have the downloadable
// fonts (SF Pro, SF Mono, New York). Safari maps sans-serif to Helvetica,
// serif to Times and monospace to Courier; Menlo is the terminal's
// monospace.
var darwinGenerics = map[string][]string{
	"system-ui":     {".SF NS", ".SF NS Text", "System Font", "SF Pro", "SF Pro Text", "Helvetica Neue", "Helvetica"},
	"ui-sans-serif": {".SF NS", ".SF NS Text", "System Font", "SF Pro", "SF Pro Text", "Helvetica Neue", "Helvetica"},
	"sans-serif":    {"Helvetica", "Helvetica Neue", "Arial"},
	"ui-serif":      {".New York", "New York", "Times", "Times New Roman"},
	"serif":         {"Times", "Times New Roman", "New York", ".New York"},
	"ui-monospace":  {".SF NS Mono", "SF Mono", "Menlo", "Monaco", "Courier New", "Courier"},
	"monospace":     {"Menlo", ".SF NS Mono", "SF Mono", "Monaco", "Courier New", "Courier"},
	"ui-rounded":    {".SF NS Rounded", "SF Pro Rounded", ".SF NS", "System Font"},
	"emoji":         {"Apple Color Emoji"},
	"cursive":       {"Apple Chancery", "Snell Roundhand", "Zapfino"},
	"fantasy":       {"Papyrus", "Chalkduster", "Herculanum"},
	"math":          {"STIX Two Math", "STIXGeneral"},
}

// windowsGenerics: Segoe UI is the system face (Segoe UI Variable, with an
// opsz axis, on Windows 11); Cambria and Cascadia Mono are the system's
// serif and monospace designs, Times New Roman and Consolas or Courier New
// the fallbacks of older systems.
var windowsGenerics = map[string][]string{
	"system-ui":     {"Segoe UI Variable Text", "Segoe UI Variable", "Segoe UI", "Tahoma", "Arial"},
	"ui-sans-serif": {"Segoe UI", "Segoe UI Variable Text", "Segoe UI Variable", "Tahoma", "Arial"},
	"sans-serif":    {"Segoe UI", "Arial", "Tahoma"},
	"ui-serif":      {"Cambria", "Times New Roman", "Georgia"},
	"serif":         {"Cambria", "Times New Roman", "Georgia"},
	"ui-monospace":  {"Cascadia Mono", "Consolas", "Courier New"},
	"monospace":     {"Cascadia Mono", "Consolas", "Courier New"},
	"emoji":         {"Segoe UI Emoji", "Segoe UI Symbol"},
	"cursive":       {"Comic Sans MS", "Segoe Script", "Segoe Print"},
	"fantasy":       {"Impact", "Gabriola"},
	"math":          {"Cambria Math"},
}

// fontconfigGenerics follows fontconfig's default configuration
// (conf.d/60-latin.conf and 60-generic.conf, the lists go-text's fontscan
// generates its substitutions from), with the families the major
// distributions configure in front of them: system-ui is GNOME's and
// Ubuntu's interface fonts, then the sans-serif list; Liberation (the
// metric clones of Arial, Times New Roman and Courier New that
// 30-metric-aliases.conf substitutes for them) is listed by name.
var fontconfigGenerics = map[string][]string{
	"system-ui": {"Cantarell", "Adwaita Sans", "Ubuntu", "Noto Sans UI", "Noto Sans", "DejaVu Sans",
		"Liberation Sans", "Roboto", "Verdana", "Arial", "Nimbus Sans", "Nimbus Sans L", "Helvetica"},
	"ui-sans-serif": {"Cantarell", "Adwaita Sans", "Ubuntu", "Noto Sans UI", "Noto Sans", "DejaVu Sans",
		"Liberation Sans", "Roboto", "Verdana", "Arial", "Nimbus Sans", "Nimbus Sans L", "Helvetica"},
	"sans-serif": {"Noto Sans", "DejaVu Sans", "Liberation Sans", "Verdana", "Arial", "Albany AMT",
		"Luxi Sans", "Nimbus Sans L", "Nimbus Sans", "Helvetica", "Lucida Sans Unicode", "Ubuntu",
		"Cantarell", "Roboto", "Tahoma"},
	"ui-serif": {"Noto Serif", "DejaVu Serif", "Liberation Serif", "Times New Roman", "Thorndale AMT",
		"Luxi Serif", "Nimbus Roman No9 L", "Nimbus Roman", "Times"},
	"serif": {"Noto Serif", "DejaVu Serif", "Liberation Serif", "Times New Roman", "Thorndale AMT",
		"Luxi Serif", "Nimbus Roman No9 L", "Nimbus Roman", "Times"},
	"ui-monospace": {"Noto Sans Mono", "DejaVu Sans Mono", "Liberation Mono", "Ubuntu Mono", "Inconsolata",
		"Andale Mono", "Courier New", "Cumberland AMT", "Luxi Mono", "Nimbus Mono L", "Nimbus Mono",
		"Nimbus Mono PS", "Courier", "Droid Sans Mono", "Cutive Mono"},
	"monospace": {"Noto Sans Mono", "DejaVu Sans Mono", "Liberation Mono", "Ubuntu Mono", "Inconsolata",
		"Andale Mono", "Courier New", "Cumberland AMT", "Luxi Mono", "Nimbus Mono L", "Nimbus Mono",
		"Nimbus Mono PS", "Courier", "Droid Sans Mono", "Cutive Mono"},
	"emoji": {"Noto Color Emoji", "Apple Color Emoji", "Segoe UI Emoji", "Twemoji", "Twitter Color Emoji",
		"EmojiOne Mozilla", "Emoji Two", "JoyPixels", "Emoji One", "Noto Emoji", "Android Emoji"},
	"cursive": {"ITC Zapf Chancery Std", "Zapfino", "Comic Sans MS", "URW Chancery L", "Z003", "TeX Gyre Chorus"},
	"fantasy": {"Impact", "Copperplate Gothic Std", "Cooper Std", "Bauhaus Std"},
	"math":    {"XITS Math", "STIX Two Math", "Cambria Math", "Latin Modern Math", "Minion Math", "Lucida Math", "Asana Math"},
}

// Generic names that the desktop's families (SystemFontOptions.UIFamily,
// MonoFamily) lead.
var (
	uiGenerics   = []string{"system-ui", "ui-sans-serif", "sans-serif"}
	monoGenerics = []string{"ui-monospace", "monospace"}
)

// systemGenerics returns the generic family lists of goos ("" is
// runtime.GOOS; operating systems other than macOS and Windows use
// fontconfig's), normalized, with ui and mono (the desktop's families,
// when not empty) first for their generic names. It makes no OS call.
func systemGenerics(goos, ui, mono string) map[string][]string {
	if goos == "" {
		goos = runtime.GOOS
	}
	lists := fontconfigGenerics
	switch goos {
	case "darwin", "ios":
		lists = darwinGenerics
	case "windows":
		lists = windowsGenerics
	}
	out := make(map[string][]string, len(lists))
	for g, names := range lists {
		var first string
		switch {
		case ui != "" && slices.Contains(uiGenerics, g):
			first = ui
		case mono != "" && slices.Contains(monoGenerics, g):
			first = mono
		}
		var l []string
		seen := map[string]bool{}
		for _, n := range append([]string{first}, names...) {
			if k := normalizeFamily(n); k != "" && !seen[k] {
				seen[k] = true
				l = append(l, k)
			}
		}
		out[g] = l
	}
	return out
}
