package main

import (
	"testing"

	"github.com/kuronosu/tela/telatest"
)

// The pages of phase 20's new pieces, whole, in each language: the window is
// as tall as the page, so the golden shows every demo. English also in the
// dark scheme where the page has colors of its own (tones, avatars, bars).
var newPages = []struct {
	name   string
	height int
	dark   bool
	// still is a page that animates for ever on purpose (a skeleton pulses,
	// an indeterminate bar slides): it is captured at its first frame.
	still bool
}{
	{"Alert", 1150, true, false},
	{"Avatar", 1500, true, false},
	{"Badge", 1050, true, false},
	{"Button", 950, false, false},
	{"ContextMenu", 900, false, false},
	{"DropdownMenu", 900, false, false},
	{"Field", 1950, false, false},
	{"Input", 1400, false, false},
	{"Menubar", 650, false, false},
	{"Progress", 950, true, true},
	{"Separator", 1150, true, false},
	{"Skeleton", 1250, true, true},
	{"Table", 2650, true, false},
	{"Toast", 600, false, false},
	{"ToggleGroup", 950, true, false},
	{"Tooltip", 800, false, false},
}

// TestGoldenNewPages renders each page in English, Spanish and Arabic
// (light, right to left in Arabic) and the dark English one:
// `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGoldenNewPages ./examples/kit`
// writes the pictures, which must be looked at before they are committed.
func TestGoldenNewPages(t *testing.T) {
	for _, p := range newPages {
		run := func(t *testing.T, tag string, dark bool) {
			opts := []telatest.Option{telatest.Size(1280, float32(p.height))}
			if dark {
				opts = append(opts, telatest.Dark())
			}
			if p.still {
				opts = append(opts, telatest.NoSettle())
			}
			a := mount(t, Canvas{Page: p.name, Locale: tag}, opts...)
			name := "kit_page_" + slug(p.name) + "_" + tag
			if dark {
				name += "_dark"
			}
			a.GoldenScales(name, 1)
		}
		t.Run(p.name, func(t *testing.T) {
			for _, tag := range languages {
				t.Run(tag, func(t *testing.T) { run(t, tag, false) })
			}
			if p.dark {
				t.Run("en dark", func(t *testing.T) { run(t, "en", true) })
			}
		})
	}
}

// The pages that phase 21 translated, whole, in Arabic: right to left, and
// with no English sentence left in a stage (the captions above each stage
// stay English and left to right on purpose). The Spanish and English pages
// are checked by TestCatalogsHaveEveryText and the page tests.
var arabicPages = []struct {
	name   string
	height int
}{
	{"AlertDialog", 650}, {"Calendar", 2500}, {"Card", 1150}, {"Checkbox", 850},
	{"ColorInput", 650}, {"ColorPicker", 1050}, {"DatePicker", 750}, {"DateRangePicker", 650},
	{"Dialog", 600}, {"FieldSet", 900}, {"Introduction", 1900}, {"Label", 600},
	{"Popover", 600}, {"RadioGroup", 800}, {"RangeCalendar", 1250}, {"Select", 650},
	{"Slider", 700}, {"Switch", 750}, {"Tabs", 950}, {"Textarea", 750},
	{"TitleBar", 2200}, {"Toggle", 550},
}

// TestGoldenArabicPages renders each of them in Arabic:
// `TELA_UPDATE=1 CGO_ENABLED=0 go test -run TestGoldenArabicPages ./examples/kit`
// writes the pictures, which must be looked at before they are committed.
func TestGoldenArabicPages(t *testing.T) {
	for _, p := range arabicPages {
		t.Run(p.name, func(t *testing.T) {
			a := mount(t, Canvas{Page: p.name, Locale: "ar"}, telatest.Size(1280, float32(p.height)))
			a.GoldenScales("kit_page_"+slug(p.name)+"_ar", 1)
		})
	}
}
