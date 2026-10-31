package main

import (
	"fmt"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
	. "github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"

	"github.com/kuronosu/tela/examples/settings/lucide"
)

type form struct {
	p     Prefs
	gap   string // the saved density: the space between blocks
	edit  func(func(p *Prefs))
	toast kit.ToastHandle
}

// stack puts the cards of a section in a column, as loose as the density.
func (f form) stack(children ...tela.Child) tela.Node {
	return Div(append([]tela.Child{Class("flex flex-col " + f.gap)}, children...)...)
}

func (f form) switchRow(label, hint string, on bool, set func(p *Prefs, on bool)) tela.Node {
	return kit.Field{
		Orientation: kit.Horizontal, ControlAfter: true, Label: label, Description: hint,
		Class:    "items-center gap-6 border-t border-border py-4 first:border-t-0 first:pt-0 last:pb-0",
		Children: kit.Switch{Checked: &on, OnCheckedChange: func(v bool) { f.edit(func(p *Prefs) { set(p, v) }) }},
	}
}

// pickLibrary asks the OS for a folder. A cancel gives "" and no error; a
// platform with no chooser says so in a toast.
func (f form) pickLibrary() {
	err := desktop.PickFolder(func(path string, err error) {
		if err != nil {
			f.toast.Error("Could not choose a folder.", kit.ToastDescription(err.Error()))
		} else if path != "" {
			f.edit(func(p *Prefs) { p.Library = path })
		}
	}, desktop.Title("Choose the library folder"))
	if err != nil {
		f.toast.Error("This system has no folder chooser.", kit.ToastDescription(err.Error()))
	}
}

func (f form) general() tela.Node {
	p := f.p
	imports := []kit.RadioItem[string]{
		{Value: "copy", Label: "Copy into the library", Description: "Originals stay on the card; the library keeps its own."},
		{Value: "leave", Label: "Leave files where they are", Description: "Darkroom only points at them. Moving a folder breaks the link."},
	}
	langs := []kit.SelectItem[string]{
		{Value: "en", Label: "English"}, {Value: "es", Label: "Español"},
		{Value: "de", Label: "Deutsch"}, {Value: "ja", Label: "日本語"},
	}
	lib := p.Library
	return f.stack(
		kit.Card{TitleLevel: 2, Title: Text("Library"), Description: Text("One folder holds every negative, print and sidecar."),
			Children: Div(Class("flex flex-col gap-6"),
				kit.Field{Label: "Library folder", Description: "Read-only here. Changing it does not move existing files.",
					Children: Div(Class("flex gap-2"),
						kit.Input{Class: "flex-1 font-mono text-[13px]", Value: &lib, ReadOnly: true, AriaLabel: "Library folder path"},
						kit.Button{Variant: kit.Outline, Icon: lucide.FolderOpen, Children: Text("Change…"), OnClick: f.pickLibrary})},
				kit.FieldSet{Legend: "When photos are imported", LegendVariant: kit.LegendVariantLabel,
					Children: kit.RadioGroup[string]{Items: imports, Value: &p.Import,
						OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Import = v }) }}},
				f.switchRow("Skip duplicates", "Frames already in the library are left out of the import.", p.SkipDuplicates,
					func(p *Prefs, v bool) { p.SkipDuplicates = v }))},
		kit.Card{TitleLevel: 2, Title: Text("Language"), Description: Text("Menus, dates and messages."),
			Children: kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center gap-6",
				Label: "Interface language", Description: "Takes effect the next time Darkroom starts.",
				Children: kit.Select[string]{Items: langs, Class: "w-44", Value: &p.Language,
					OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Language = v }) }}}},
	)
}

var looks = []struct {
	Key, Label, Note string
	Strip            []string
}{
	{"neutral", "Neutral", "Flat and honest.", []string{"#3a3a3a", "#6b6b6b", "#9a9a9a", "#c8c8c8", "#ececec"}},
	{"portra", "Portra", "Soft skin, warm shadows.", []string{"#4a3a34", "#8a6a58", "#c99a7b", "#e8c9a6", "#f6e6cf"}},
	{"trix", "Tri-X", "Grain, contrast, black and white.", []string{"#050505", "#2b2b2b", "#6e6e6e", "#b4b4b4", "#f0f0f0"}},
	{"velvia", "Velvia", "Saturated, a little theatrical.", []string{"#1b2a63", "#0f7a4f", "#e0a417", "#d8362b", "#8c2a8f"}},
}

func (f form) develop() tela.Node {
	p := f.p
	items := []kit.RadioItem[string]{}
	for _, l := range looks {
		items = append(items, kit.RadioItem[string]{Value: l.Key, Label: l.Label, Description: l.Note})
	}
	strips := map[string][]string{}
	for _, l := range looks {
		strips[l.Key] = l.Strip
	}
	return f.stack(
		kit.Card{TitleLevel: 2, Title: Text("Default look"), Description: Text("Applied to every new photo. Existing photos are left alone."),
			Children: kit.RadioGroup[string]{AriaLabel: "Default look", Items: items, Value: &p.Look, Class: "grid-cols-2",
				OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Look = v }) },
				Render: func(o kit.RadioItem[string], st kit.RadioState) tela.Node {
					return optionCard(o, st, Div(Class("mb-1 flex h-7 w-full overflow-hidden rounded-md ring-1 ring-border"),
						Map(strips[o.Value], func(c string) tela.Node {
							return Div(Key(c), Class("h-full flex-1 bg-["+c+"]"))
						})))
				}}},
		kit.Card{TitleLevel: 2, Title: Text("Detail"), Description: Text("The first pass of sharpening and noise."),
			Children: Div(Class("flex flex-col gap-6"),
				f.sliderRow("Sharpening", "Amount", p.Sharpen, 0, 150, "%.0f", func(p *Prefs, v float64) { p.Sharpen = v }),
				f.sliderRow("Noise reduction", "Luminance", p.Noise, 0, 100, "%.0f", func(p *Prefs, v float64) { p.Noise = v }))},
		kit.Card{TitleLevel: 2, Title: Text("Corrections"),
			Children: Div(Class("flex flex-col"),
				f.switchRow("Lens corrections", "Distortion and vignetting, from the lens profile.", p.Lens, func(p *Prefs, v bool) { p.Lens = v }),
				f.switchRow("Auto-straighten horizons", "Levels a tilted horizon up to five degrees.", p.Straighten, func(p *Prefs, v bool) { p.Straighten = v }))},
	)
}

func (f form) sliderRow(label, hint string, v, lo, hi float64, format string, set func(p *Prefs, v float64)) tela.Node {
	return Div(Class("flex flex-col gap-3"),
		Div(Class("flex items-baseline justify-between"),
			Div(Class("flex flex-col gap-0.5"),
				Span(Class("text-sm leading-none font-medium"), Text(label)),
				Span(Class("text-sm text-muted-foreground"), Text(hint))),
			Span(Class("text-sm font-medium tabular-nums"), Textf(format, v))),
		kit.Slider{AriaLabel: label, Min: lo, Max: hi, Step: 1, Value: &v,
			OnValueChange: func(x float64) { f.edit(func(p *Prefs) { set(p, x) }) }})
}

// optionCard draws a radio as a card; extra is shown above the label.
func optionCard(o kit.RadioItem[string], st kit.RadioState, extra tela.Node) tela.Node {
	return Button(Key(o.Value), st.Props,
		Class("flex flex-col items-start gap-1.5 rounded-lg border border-border bg-card p-3.5 text-left shadow-xs transition-colors outline-none",
			"hover:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50",
			"aria-checked:border-primary aria-checked:bg-primary/10 aria-checked:ring-1 aria-checked:ring-primary"),
		extra,
		Div(Class("flex items-center gap-2 text-sm font-medium"), st.Indicator, Text(o.Label)),
		P(Class("text-xs leading-snug text-muted-foreground"), Text(o.Description)))
}

func (f form) export() tela.Node {
	p := f.p
	formats := []kit.RadioItem[string]{
		{Value: "jpeg", Label: "JPEG", Description: "Small and universal. Lossy."},
		{Value: "png", Label: "PNG", Description: "Lossless, with transparency."},
		{Value: "tiff", Label: "TIFF", Description: "Lossless 16-bit, for print."},
		{Value: "heic", Label: "HEIC", Description: "Half the size of JPEG, newer apps only."},
	}
	spaces := []kit.SelectItem[string]{
		{Value: "srgb", Label: "sRGB"}, {Value: "p3", Label: "Display P3"}, {Value: "adobe", Label: "Adobe RGB"},
	}
	sizes := []kit.ToggleGroupItem[string]{
		{Value: "off", Label: "Off"}, {Value: "2048", Label: "2048"}, {Value: "4096", Label: "4096"}, {Value: "custom", Label: "Custom"},
	}
	colors := []kit.RadioItem[string]{}
	for _, k := range []string{"white", "black", "red"} {
		colors = append(colors, kit.RadioItem[string]{Value: k, Label: map[string]string{"white": "White", "black": "Black", "red": "Safelight"}[k]})
	}
	lossy := p.Format == "jpeg" || p.Format == "heic"
	cerr := customError(p)

	var quality tela.Node = Div(Class("rounded-md border border-dashed border-border px-3 py-2.5 text-sm text-muted-foreground"),
		Textf("%s is lossless: there is no quality to set.", map[string]string{"png": "PNG", "tiff": "TIFF"}[p.Format]))
	if lossy {
		quality = f.sliderRow("Quality", "Higher is larger and sharper", p.Quality, 1, 100, "%.0f", func(p *Prefs, v float64) { p.Quality = v })
	}
	custom := tela.Node(Fragment())
	if p.Resize == "custom" {
		custom = kit.Field{Label: "Long edge (px)", Description: "From 256 to 16384.", Error: cerr,
			Children: kit.Input{Class: "w-40", Type: "text", Value: &p.Custom, Invalid: cerr != "",
				OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Custom = v }) }}}
	}
	wm := tela.Node(Fragment())
	if p.Watermark {
		wmColor := wmHex[p.WmColor]
		wm = Div(Class("mt-4 flex flex-col gap-5"),
			kit.Field{Label: "Watermark text", Children: kit.Input{Value: &p.WmText,
				OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.WmText = v }) }}},
			kit.FieldSet{Legend: "Watermark color", LegendVariant: kit.LegendVariantLabel,
				Children: kit.RadioGroup[string]{Items: colors, Value: &p.WmColor, Orientation: kit.Horizontal,
					OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.WmColor = v }) }}},
			Div(Class("flex h-24 items-end justify-end rounded-lg bg-linear-to-br from-[#2d3a4a] via-[#5b4a45] to-[#14110f] p-3"),
				Span(Class("text-sm font-medium tracking-wide text-["+wmColor+"]"), Text(p.WmText))))
	}

	return f.stack(
		kit.Card{TitleLevel: 2, Title: Text("Format"), Description: Text("What a finished frame is written as."),
			Children: Div(Class("flex flex-col gap-6"),
				kit.RadioGroup[string]{AriaLabel: "File format", Items: formats, Value: &p.Format, Class: "grid-cols-2",
					OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Format = v }) },
					Render:        func(o kit.RadioItem[string], st kit.RadioState) tela.Node { return optionCard(o, st, Fragment()) }},
				quality,
				kit.Field{Orientation: kit.Horizontal, ControlAfter: true, Class: "items-center gap-6",
					Label: "Color space", Description: "sRGB is safest for the web.",
					Children: kit.Select[string]{Items: spaces, Class: "w-44", Value: &p.ColorSpace,
						OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.ColorSpace = v }) }}})},
		kit.Card{TitleLevel: 2, Title: Text("Size"), Description: Text("Scale the long edge on export."),
			Children: Div(Class("flex flex-col gap-5"),
				kit.FieldSet{Legend: "Resize long edge", LegendVariant: kit.LegendVariantLabel,
					Children: kit.ToggleGroup[string]{Variant: kit.Outline, DisallowEmptySelection: true, Value: &p.Resize,
						Items: sizes, OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Resize = v }) }}},
				custom)},
		kit.Card{TitleLevel: 2, Title: Text("Watermark"),
			Children: Div(Class("flex flex-col"),
				f.switchRow("Stamp exported frames", "A line of text in the lower corner.", p.Watermark, func(p *Prefs, v bool) { p.Watermark = v }),
				wm)},
	)
}

var wmHex = map[string]string{"white": "#ffffff", "black": "#000000", "red": "#e5412f"}

var thumbGrads = []string{
	"from-[#2d3a4a] to-[#c98f6a]", "from-[#1f3b2e] to-[#d9c58a]", "from-[#3a2438] to-[#e08a6b]",
	"from-[#10202e] to-[#6aa5c9]", "from-[#46352a] to-[#e9d8b4]", "from-[#22222a] to-[#8f8fa8]",
}

func themeMini(bg, side, line, ink string) tela.Node {
	return Div(Class("flex h-full min-w-0 flex-1 bg-["+bg+"]"),
		Div(Class("flex w-1/4 flex-col gap-1 bg-["+side+"] p-1.5"),
			Div(Class("h-1 w-full rounded-full bg-["+line+"]")), Div(Class("h-1 w-2/3 rounded-full bg-["+line+"]"))),
		Div(Class("flex flex-1 flex-col gap-1.5 p-2"),
			Div(Class("h-1.5 w-1/2 rounded-full bg-["+ink+"]")),
			Div(Class("h-6 w-full rounded-sm bg-["+side+"]")),
			Div(Class("h-1 w-3/4 rounded-full bg-["+line+"]"))))
}

func (f form) safelight() tela.Node {
	p := f.p
	themes := []kit.RadioItem[string]{
		{Value: "light", Label: "Light", Description: "Paper white, for bright rooms."},
		{Value: "dark", Label: "Dark", Description: "The natural habitat."},
		{Value: "system", Label: "System", Description: "Follows the OS."},
	}
	light := themeMini("#f4f0ea", "#e4ddd2", "#c9bfb0", "#6f655c")
	dark := themeMini("#141110", "#241f1c", "#3a322d", "#a0948a")
	minis := map[string]tela.Node{"light": light, "dark": dark, "system": Fragment(light, dark)}
	accItems := []kit.RadioItem[string]{}
	for _, a := range accents {
		accItems = append(accItems, kit.RadioItem[string]{Value: a.Key, Label: a.Label})
	}
	accHex := map[string]tela.Color{}
	for _, a := range accents {
		accHex[a.Key] = a.Color
	}
	acc := accentKey(p.Accent)
	dens := []kit.ToggleGroupItem[string]{
		{Value: "compact", Label: "Compact"}, {Value: "default", Label: "Default"}, {Value: "comfortable", Label: "Comfortable"},
	}
	sz := int(p.Thumb)
	return f.stack(
		kit.Card{TitleLevel: 2, Title: Text("Theme"), Description: Text("Applied the moment you save."),
			Children: kit.RadioGroup[string]{AriaLabel: "Theme", Items: themes, Value: &p.Theme, Class: "grid-cols-3",
				OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Theme = v }) },
				Render: func(o kit.RadioItem[string], st kit.RadioState) tela.Node {
					return optionCard(o, st, Div(Class("mb-1 flex h-16 w-full overflow-hidden rounded-md ring-1 ring-border"), minis[o.Value]))
				}}},
		kit.Card{TitleLevel: 2, Title: Text("Accent"), Description: Text("One color, used sparingly. Safelight red by default."),
			Children: Div(Class("flex flex-wrap items-center gap-x-6 gap-y-4"),
				kit.RadioGroup[string]{AriaLabel: "Accent presets", Items: accItems, Value: &acc, Orientation: kit.Horizontal,
					OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Accent = accHex[v] }) },
					Render: func(o kit.RadioItem[string], st kit.RadioState) tela.Node {
						return Button(Key(o.Value), st.Props, AriaLabel(o.Label), Style(tela.Bg(accHex[o.Value])),
							Class("flex size-9 items-center justify-center rounded-full text-white shadow-xs outline-none transition-transform",
								"hover:scale-110 focus-visible:ring-[3px] focus-visible:ring-ring/50",
								"aria-checked:ring-2 aria-checked:ring-foreground aria-checked:ring-offset-2 aria-checked:ring-offset-card"),
							If(st.Checked, Icon(lucide.Check, Class("size-4"))))
					}},
				Div(Class("flex items-center gap-2 text-sm text-muted-foreground"), Text("Custom"),
					kit.ColorInput{AriaLabel: "Custom accent color", Value: &p.Accent, OnValueChange: func(c tela.Color) { f.edit(func(p *Prefs) { p.Accent = c }) }}))},
		kit.Card{TitleLevel: 2, Title: Text("Interface density"),
			Children: kit.ToggleGroup[string]{AriaLabel: "Interface density", Variant: kit.Outline, DisallowEmptySelection: true, Value: &p.Density, Items: dens,
				OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Density = v }) }}},
		kit.Card{TitleLevel: 2, Title: Text("Contact sheet"), Description: Text("How large thumbnails are drawn."),
			Children: Div(Class("flex flex-col gap-5"),
				f.sliderRow("Thumbnail size", "In pixels", p.Thumb, 96, 256, "%.0f px", func(p *Prefs, v float64) { p.Thumb = v }),
				Div(Class("flex h-[300px] flex-wrap content-start gap-3 overflow-hidden rounded-lg bg-muted p-4"), AriaHidden(true),
					Range(6, func(i int) tela.Node {
						return Div(Class(fmt.Sprintf("flex shrink-0 items-end rounded-md bg-linear-to-br shadow-sm %s h-[%dpx] w-[%dpx]", thumbGrads[i], sz*3/4, sz)),
							Span(Class("m-1.5 text-[10px] text-white/80 tabular-nums"), Textf("IMG_%04d", 1204+i)))
					})))},
	)
}

func (f form) account() tela.Node {
	p := f.p
	eerr := emailError(p.Email)
	return f.stack(
		kit.Card{TitleLevel: 2, Title: Text("Profile"), Description: Text("How you appear on shared contact sheets."),
			Children: Div(Class("flex flex-col gap-5"),
				kit.Field{Label: "Name", Children: kit.Input{Value: &p.Name, OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Name = v }) }}},
				kit.Field{Label: "E-mail", Description: "For receipts and sign-in. Never shared.", Error: eerr,
					Children: kit.Input{Type: "email", Value: &p.Email, Invalid: eerr != "",
						OnValueChange: func(v string) { f.edit(func(p *Prefs) { p.Email = v }) }}})},
		kit.Card{TitleLevel: 2, Title: Text("Plan"),
			Action:      kit.Badge{Icon: lucide.Gem, Children: Text("Fixer plan")},
			Description: Text("Unlimited develops, 100 GB of archive."),
			Children: Div(Class("flex flex-col gap-3"),
				Div(Class("flex items-baseline justify-between text-sm"),
					Span(Class("font-medium"), Text("Storage used")),
					Span(Class("text-muted-foreground tabular-nums"), Text("41.2 GB of 100 GB"))),
				kit.Progress{Value: 41.2, Max: 100, AriaLabel: "Storage used"},
				P(Class("text-xs text-muted-foreground"), Text("58.8 GB free. Originals count; previews do not.")))},
		kit.Card{TitleLevel: 2, Title: Text("Devices"),
			Children: Div(Class("flex items-center justify-between gap-6"),
				Div(Class("flex flex-col gap-1.5"),
					Span(Class("text-sm leading-snug font-medium"), Text("Sign out of all devices")),
					Span(Class("text-sm text-muted-foreground"), Text("Every other computer and phone will need to sign in again."))),
				kit.AlertDialog{
					Trigger:     kit.Button{Variant: kit.Outline, Icon: lucide.LogOut, Children: Text("Sign out everywhere")},
					Title:       Text("Sign out of all devices?"),
					Description: Text("You will stay signed in here. Other devices lose access until you sign in again."),
					Action:      Text("Sign out"), Destructive: true,
					OnAction: func() { f.toast.Info("Signed out of all devices.") },
				})},
	)
}
