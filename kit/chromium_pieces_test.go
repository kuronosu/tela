package kit_test

import (
	"fmt"
	"html"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// Chromium conformance of the pieces phase 20 added (ADR-248) and phase 21
// extended (ADR-256): Alert, Badge tones and dot, Button loading, Input
// icons, Separator caption, AvatarGroup, Field slots and Table sort states.
//
// shadcn/ui has the Alert, so its markup is the reference where it can be.
// The rest are Tela's additions, with no markup of shadcn's to follow: the
// Chromium side writes the same structure by hand with the classes of the
// default recipe (kit.DefaultStyle), which the page's Tailwind 4.3.3 turns
// into CSS. What is compared is the engine, Tela's layout and paint of those
// classes against Chromium's, plus the structure of each piece.
//
// The sheets are those of the goldens (their names), restricted to the cells
// that have markup; regenerate the pages as harness_test.go says.

// withMarkup keeps the cells of s that have an entry in markup and gives
// them that markup.
func (s *sheet) withMarkup(markup map[string]string) *sheet {
	var cells []cell
	for _, c := range s.cells {
		if m, ok := markup[c.name]; ok {
			c.html = m
			cells = append(cells, c)
		}
	}
	s.cells = cells
	return s
}

// noSpin freezes the spinners in Chromium's page: a screenshot of a running
// animation is a different angle each time, and Tela's first frame is at 0.
const noSpin = ".animate-spin { animation: none !important; }"

// iconLoader is Lucide's loader-circle, the spinner of a loading Button.
var iconLoader = tela.Icon{SVG: `<svg xmlns="http://www.w3.org/2000/svg" width="24" height="24" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M21 12a9 9 0 1 1-6.219-8.56"/></svg>`}

// ---- Alert ----------------------------------------------------------------

// shadcn/ui's Alert (new-york-v4/ui/alert.tsx at kit.ShadcnCommit), without
// the destructive variant's descendant selector, which the description's own
// class stands for.
const (
	calloutRoot       = "relative grid w-full grid-cols-[0_1fr] items-start gap-y-0.5 rounded-lg border px-4 py-3 text-sm has-[>svg]:grid-cols-[calc(var(--spacing)*4)_1fr] has-[>svg]:gap-x-3 [&>svg]:size-4 [&>svg]:translate-y-0.5 [&>svg]:text-current"
	calloutTitle      = "col-start-2 line-clamp-1 min-h-4 font-medium tracking-tight"
	calloutDesc       = "col-start-2 grid justify-items-start gap-1 text-sm"
	alertActionHTMLPE = "pe-18"
)

// alertHTML is the markup of an alert named name; icon, title, desc and
// action are HTML ("" for none).
func alertHTML(name string, v kit.Variant, icon tela.Icon, title, desc, action string) string {
	s := kit.DefaultStyle().Alert
	variant := s.Variants[v]
	descColor := "text-muted-foreground"
	switch v {
	case kit.Default:
	case kit.Destructive:
		descColor = "text-destructive/90"
	default:
		descColor = "text-current/80"
	}
	svg := ""
	if icon != (tela.Icon{}) {
		svg = svgHTML(icon)
	}
	if title != "" {
		title = fmt.Sprintf(`<div data-slot="alert-title" class="%s">%s</div>`, calloutTitle, title)
	}
	if desc != "" {
		desc = fmt.Sprintf(`<div data-slot="alert-description" class="%s">%s</div>`, cn(calloutDesc, descColor), desc)
	}
	root := cn(calloutRoot, variant)
	if action != "" {
		root = cn(root, alertActionHTMLPE)
		action = fmt.Sprintf(`<div class="absolute end-3 top-2.5">%s</div>`, action)
	}
	return fmt.Sprintf(`<div data-slot="alert" data-probe="%s-alert" role="alert" class="%s">%s%s%s%s</div>`, name, root, svg, title, desc, action)
}

func alertChromium() *sheet {
	wrapped := `<div class="flex flex-col gap-1"><p>Please verify your billing information and try again.</p><div class="flex flex-col text-sm"><div>Check your card details</div><div>Ensure sufficient funds</div></div></div>`
	s := alertSheet().withMarkup(map[string]string{
		"default":          alertHTML("default", kit.Default, iconTerminal, "Heads up!", "You can add components to your app using the cli.", ""),
		"destructive":      alertHTML("destructive", kit.Destructive, iconCircleAlert, "Unable to process your payment.", "Please verify your billing information and try again.", ""),
		"no-icon":          alertHTML("no-icon", kit.Default, tela.Icon{}, "Your session has expired", "Log in again to keep working.", ""),
		"title-only":       alertHTML("title-only", kit.Default, iconInfo, "A new version is available", "", ""),
		"description-only": alertHTML("description-only", kit.Default, iconInfo, "", "This account was created with a different sign-in method.", ""),
		"wrapped":          alertHTML("wrapped", kit.Destructive, iconCircleAlert, "Unable to process your payment.", wrapped, ""),
		"success":          alertHTML("success", kit.Success, iconCircleCheck, "Payment received", "A receipt is on its way to your inbox.", ""),
		"warning":          alertHTML("warning", kit.Warning, iconTriangleAlert, "Your trial ends in 3 days", "Add a payment method to keep your projects.", ""),
		"info":             alertHTML("info", kit.Info, iconInfo, "Scheduled maintenance", "The service is read-only on Sunday from 02:00 to 03:00 UTC.", ""),
	})
	return s
}

func TestChromiumAlert(t *testing.T) { alertChromium().chromium(t) }

func alertActionChromium() *sheet {
	restart := func(label string) string {
		return buttonHTML(kit.Button{Size: kit.Xs, Variant: kit.Outline}, label, "")
	}
	return alertActionSheet().withMarkup(map[string]string{
		"default": alertHTML("default", kit.Default, iconInfo, "Update available", "Restart to install version 2.4.", restart("Restart")),
		"destructive": alertHTML("destructive", kit.Destructive, iconCircleAlert, "Upload failed", "The file is larger than 10 MB.",
			restart("Retry")),
		"dismiss": alertHTML("dismiss", kit.Info, iconInfo, "Scheduled maintenance", "The service is read-only on Sunday from 02:00 to 03:00 UTC.",
			buttonHTML(kit.Button{Size: kit.IconXs, Variant: kit.Ghost}, svgHTML(iconX), "")),
		"title-only": alertHTML("title-only", kit.Default, tela.Icon{}, "A new version is available", "",
			buttonHTML(kit.Button{Size: kit.Xs}, "Update", "")),
	})
}

func TestChromiumAlertAction(t *testing.T) { alertActionChromium().chromium(t) }

func TestChromiumAlertActionRTL(t *testing.T) {
	s := alertActionChromium()
	s.name, s.rtl = "alert_action_rtl", true
	s.cells = s.cells[:3]
	s.chromium(t)
}

// ---- Badge tones and dot ----------------------------------------------------

func toneBadgeHTML(name string, v kit.Variant, icon tela.Icon, dot bool, dotClass, text string) string {
	s := kit.DefaultStyle().Badge
	variant := shadcnBadgeVariants[v]
	if t, ok := s.Variants[v]; ok && variant == "" {
		variant = t
	}
	inner := ""
	if icon != (tela.Icon{}) {
		inner += svgHTML(icon)
	}
	if dot {
		dc := s.Dot
		if dotClass != "" {
			dc = strings.Replace(dc, "bg-current", dotClass, 1)
		}
		inner += fmt.Sprintf(`<span aria-hidden="true" class="%s"></span>`, dc)
	}
	return fmt.Sprintf(`<span data-probe="%s-badge" class="%s">%s%s</span>`, name, cn(shadcnBadge, variant), inner, text)
}

func badgeTonesChromium() *sheet {
	b := toneBadgeHTML
	return badgeTonesSheet().withMarkup(map[string]string{
		"success":           b("success", kit.Success, tela.Icon{}, false, "", "Paid"),
		"warning":           b("warning", kit.Warning, tela.Icon{}, false, "", "Pending"),
		"info":              b("info", kit.Info, tela.Icon{}, false, "", "Draft"),
		"success-icon":      b("success-icon", kit.Success, iconCheck, false, "", "Verified"),
		"warning-icon":      b("warning-icon", kit.Warning, iconCircleAlert, false, "", "Review"),
		"dot-success":       b("dot-success", kit.Success, tela.Icon{}, true, "", "Active"),
		"dot-warning":       b("dot-warning", kit.Warning, tela.Icon{}, true, "", "Pending"),
		"dot-info":          b("dot-info", kit.Info, tela.Icon{}, true, "", "Syncing"),
		"dot-destructive":   b("dot-destructive", kit.Destructive, tela.Icon{}, true, "", "Failed"),
		"dot-default":       b("dot-default", kit.Default, tela.Icon{}, true, "", "Default"),
		"dot-secondary":     b("dot-secondary", kit.Secondary, tela.Icon{}, true, "", "Idle"),
		"dot-outline":       b("dot-outline", kit.Outline, tela.Icon{}, true, "bg-emerald-500", "Online"),
		"dot-outline-amber": b("dot-outline-amber", kit.Outline, tela.Icon{}, true, "bg-amber-500", "Away"),
		"dot-icon":          b("dot-icon", kit.Info, iconCheck, true, "", "Both"),
	})
}

func TestChromiumBadgeTones(t *testing.T) { badgeTonesChromium().chromium(t) }

// ---- Button loading ----------------------------------------------------------

// loadingHTML is a loading button: lead and trail say where the spinner is
// ("lead", "trail", or "" for the overlay).
func loadingHTML(name string, b kit.Button, label, where string) string {
	size := "size-4"
	if b.Size == kit.Xs || b.Size == kit.IconXs {
		size = "size-3"
	}
	spinner := svgWithClass(iconLoader, "pointer-events-none shrink-0 "+size+" animate-spin")
	var inner string
	class := "pointer-events-none"
	switch where {
	case "lead":
		inner = spinner + label
	case "trail":
		inner = label + spinner
	default:
		class += " relative"
		inner = fmt.Sprintf(`<span class="contents text-transparent">%s</span><span aria-hidden="true" class="absolute inset-0 flex items-center justify-center">%s</span>`, label, spinner)
	}
	b.Class = class
	return strings.Replace(buttonHTML(b, inner, name+"-button"), "<button", `<button type="button" aria-busy="true"`, 1)
}

func idleHTML(name string, b kit.Button, inner string) string {
	return buttonHTML(b, inner, name+"-button")
}

func buttonLoadingChromium() *sheet {
	s := buttonLoadingSheet().withMarkup(map[string]string{
		"text":        loadingHTML("text", kit.Button{}, "Save changes", ""),
		"text-idle":   idleHTML("text-idle", kit.Button{}, "Save changes"),
		"icon":        loadingHTML("icon", kit.Button{}, "Login with email", "lead"),
		"icon-idle":   idleHTML("icon-idle", kit.Button{}, svgHTML(iconMail)+"Login with email"),
		"icon-end":    loadingHTML("icon-end", kit.Button{Variant: kit.Secondary}, "Continue", "trail"),
		"outline":     loadingHTML("outline", kit.Button{Variant: kit.Outline}, "Submit", ""),
		"destructive": loadingHTML("destructive", kit.Button{Variant: kit.Destructive}, "Delete", "lead"),
		"ghost":       loadingHTML("ghost", kit.Button{Variant: kit.Ghost}, "Refresh", ""),
		"sm":          loadingHTML("sm", kit.Button{Size: kit.Sm, Variant: kit.Outline}, "Small", ""),
		"lg":          loadingHTML("lg", kit.Button{Size: kit.Lg}, "Large", ""),
		"icon-only":   loadingHTML("icon-only", kit.Button{Variant: kit.Outline, Size: kit.Icon}, "", "lead"),
		"link":        loadingHTML("link", kit.Button{Variant: kit.Link}, "Resend email", ""),
	})
	s.css = noSpin
	return s
}

func TestChromiumButtonLoading(t *testing.T) { buttonLoadingChromium().chromium(t) }

// ---- Input icons ---------------------------------------------------------------

// inputAttrs are the attributes of the <input> that stand for i's fields.
func inputAttrs(i kit.Input) string {
	attrs := ""
	if i.Type != "" {
		attrs += fmt.Sprintf(` type=%q`, i.Type)
	}
	if i.Placeholder != "" {
		attrs += fmt.Sprintf(` placeholder=%q`, html.EscapeString(i.Placeholder))
	}
	if i.DefaultValue != "" {
		attrs += fmt.Sprintf(` value=%q`, html.EscapeString(i.DefaultValue))
	}
	if i.Disabled {
		attrs += " disabled"
	}
	if i.Invalid {
		attrs += ` aria-invalid="true"`
	}
	return attrs
}

// inputIconHTML is an input with icons in the recipe's wrapper; end is the
// HTML of an EndContent ("" for none).
func inputIconHTML(name string, i kit.Input, end string) string {
	s := kit.DefaultStyle().Input
	hasEnd := i.IconEnd != (tela.Icon{}) || end != ""
	ps, pe := "", ""
	if i.Icon != (tela.Icon{}) {
		ps = s.PadStart
	}
	if hasEnd {
		pe = s.PadEnd
	}
	in := fmt.Sprintf(`<input data-probe="%s-input"%s class="%s">`, name, inputAttrs(i), cn(shadcnInput, "peer", ps, pe))
	icon := func(ic tela.Icon, place string) string {
		if ic == (tela.Icon{}) {
			return ""
		}
		return svgWithClass(ic, strings.Join([]string{s.Icon, place, s.IconDisabled, s.IconInvalid}, " "))
	}
	action := ""
	if end != "" {
		action = fmt.Sprintf(`<div class="%s">%s</div>`, s.Action+" "+s.ActionDisabled, end)
	}
	return fmt.Sprintf(`<div class="%s">%s%s%s%s</div>`, s.Wrapper, in, icon(i.Icon, s.IconStart), icon(i.IconEnd, s.IconEnd), action)
}

func inputIconChromium() *sheet {
	m := inputIconHTML
	return inputIconSheet().withMarkup(map[string]string{
		"start":       m("start", kit.Input{Icon: iconSearch, Placeholder: "Search..."}, ""),
		"start-value": m("start-value", kit.Input{Icon: iconMail, DefaultValue: "m@example.com", Type: "email"}, ""),
		"end":         m("end", kit.Input{IconEnd: iconCheck, DefaultValue: "available-name"}, ""),
		"both":        m("both", kit.Input{Icon: iconSearch, IconEnd: iconX, DefaultValue: "tela"}, ""),
		"disabled":    m("disabled", kit.Input{Icon: iconSearch, Placeholder: "Search...", Disabled: true}, ""),
		"invalid":     m("invalid", kit.Input{Icon: iconMail, DefaultValue: "not an email", Invalid: true}, ""),
		"long":        m("long", kit.Input{Icon: iconSearch, DefaultValue: "a very long value that runs under the trailing icon of the field", IconEnd: iconX}, ""),
	})
}

func TestChromiumInputIcon(t *testing.T) { inputIconChromium().chromium(t) }

func TestChromiumInputIconRTL(t *testing.T) {
	s := inputIconChromium()
	s.name, s.rtl = "input_icon_rtl", true
	s.cells = s.cells[:4]
	s.chromium(t)
}

func inputEndContentChromium() *sheet {
	btn := func(ic tela.Icon) string {
		return buttonHTML(kit.Button{Variant: kit.Ghost, Size: kit.IconXs}, svgHTML(ic), "")
	}
	m := inputIconHTML
	return endContentSheet().withMarkup(map[string]string{
		"password":    m("password", kit.Input{Type: "password", DefaultValue: "correct horse"}, btn(iconEye)),
		"shown":       m("shown", kit.Input{DefaultValue: "correct horse"}, btn(iconEyeOff)),
		"placeholder": m("placeholder", kit.Input{Placeholder: "Password", Type: "password"}, btn(iconEye)),
		"start":       m("start", kit.Input{Icon: iconMail, Type: "email", DefaultValue: "m@example.com"}, btn(iconX)),
		"disabled":    m("disabled", kit.Input{Disabled: true, DefaultValue: "locked"}, btn(iconEye)),
		"invalid":     m("invalid", kit.Input{Invalid: true, DefaultValue: "not valid"}, btn(iconEye)),
	})
}

func TestChromiumInputEndContent(t *testing.T) { inputEndContentChromium().chromium(t) }

// ---- Separator caption --------------------------------------------------------

func separatorHTML(name string, o kit.Orientation, a kit.CaptionAlign, caption, captionClass string) string {
	s := kit.DefaultStyle().Separator
	if o == "" {
		o = kit.Horizontal
	}
	line := func(short bool) string {
		m := s.Line
		if short {
			m = s.LineShort
		}
		return fmt.Sprintf(`<div aria-hidden="true" class="%s"></div>`, m[o])
	}
	return fmt.Sprintf(`<div data-probe="%s-sep" class="%s">%s<div class="%s">%s</div>%s</div>`,
		name, s.Captioned[o], line(a == kit.CaptionStart), cn(s.Caption, captionClass), caption, line(a == kit.CaptionEnd))
}

// sepCell is a separator cell: the Tela side marks the separator "<name>-sep",
// as the markup does.
func sepCell(name string, sep kit.Separator, vertical bool, caption string) cell {
	sep.Props = h.TestID(name + "-sep")
	t := tela.Component(sep)
	markup := separatorHTML(name, sep.Orientation, sep.CaptionAlign, caption, sep.CaptionClass)
	if vertical {
		t = h.Div(h.Class("flex h-32 items-center gap-6 text-sm"), h.Div(h.Text("Left")), sep, h.Div(h.Text("Right")))
		markup = `<div class="flex h-32 items-center gap-6 text-sm"><div>Left</div>` + markup + `<div>Right</div></div>`
	}
	c := cell{name: name, probes: []string{name + "-sep"}, tela: t, html: markup}
	if !vertical {
		c.width = 320
	}
	return c
}

func separatorCaptionChromium() *sheet {
	return &sheet{name: "separator_caption", cells: []cell{
		sepCell("or", kit.Separator{Caption: h.Text("Or continue with")}, false, "Or continue with"),
		sepCell("uppercase", kit.Separator{Caption: h.Text("Today"), CaptionClass: "text-xs uppercase tracking-wider"}, false, "Today"),
		sepCell("vertical", kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or")}, true, "or"),
	}}
}

func TestChromiumSeparatorCaption(t *testing.T) { separatorCaptionChromium().chromium(t) }

func separatorAlignChromium() *sheet {
	return &sheet{name: "separator_align", slot: 344, cells: []cell{
		sepCell("start", kit.Separator{Caption: h.Text("Yesterday"), CaptionAlign: kit.CaptionStart}, false, "Yesterday"),
		sepCell("center", kit.Separator{Caption: h.Text("Yesterday")}, false, "Yesterday"),
		sepCell("end", kit.Separator{Caption: h.Text("Yesterday"), CaptionAlign: kit.CaptionEnd}, false, "Yesterday"),
		sepCell("vertical-start", kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or"), CaptionAlign: kit.CaptionStart}, true, "or"),
		sepCell("vertical-end", kit.Separator{Orientation: kit.Vertical, Caption: h.Text("or"), CaptionAlign: kit.CaptionEnd}, true, "or"),
	}}
}

func TestChromiumSeparatorAlign(t *testing.T) { separatorAlignChromium().chromium(t) }

// ---- AvatarGroup ------------------------------------------------------------------

// avatarMarkup is an avatar with its fallback.
func avatarMarkup(size kit.Size, color kit.AvatarColor, text, extra string) string {
	s := kit.DefaultStyle().Avatar
	root := cn(s.Root, s.Sizes[size], extra)
	fallback := cn(s.Fallback, s.FallbackSizes[size], s.Colors[color])
	if color == "" {
		fallback = cn(s.Fallback, s.FallbackSizes[size])
	}
	return fmt.Sprintf(`<div class="%s"><div class="%s">%s</div></div>`, root, fallback, text)
}

type avatarSpec struct {
	text  string
	color kit.AvatarColor
}

var teamSpec = []avatarSpec{{"MT", "red"}, {"AL", "emerald"}, {"AT", "sky"}, {"GH", "violet"}, {"LT", "amber"}, {"MH", "pink"}}

func groupCell(name string, people []avatarSpec, more int, size kit.Size, ring string, width int) cell {
	avatars := make([]kit.Avatar, len(people))
	var htmlParts []string
	g := kit.DefaultStyle().AvatarGroup
	for i, p := range people {
		avatars[i] = kit.Avatar{Fallback: p.text, Color: p.color}
		htmlParts = append(htmlParts, avatarMarkup(size, p.color, p.text, g.Avatar+" "+ring))
	}
	if more > 0 {
		htmlParts = append(htmlParts, strings.Replace(avatarMarkup(size, "", fmt.Sprintf("+%d", more), g.Avatar+" "+ring), `<div class="relative`, `<div dir="ltr" class="relative`, 1))
	}
	grp := kit.AvatarGroup{Avatars: avatars, Size: size, AvatarClass: ring, Props: h.TestID(name + "-group")}
	if more > 0 {
		grp.Total = len(people) + more
	}
	return cell{name: name, width: width, probes: []string{name + "-group"}, tela: grp,
		html: fmt.Sprintf(`<div role="group" data-probe="%s-group" class="%s">%s</div>`, name, g.Root, strings.Join(htmlParts, ""))}
}

func avatarGroupChromium() *sheet {
	return &sheet{name: "avatar_group", width: 640, cells: []cell{
		groupCell("all", teamSpec[:4], 0, "", "", 280),
		groupCell("max", teamSpec[:3], 3, "", "", 280),
		groupCell("total", teamSpec[:3], 9, "", "", 280),
		groupCell("sm", teamSpec[:4], 2, kit.Sm, "", 280),
		groupCell("lg", teamSpec[:4], 2, kit.Lg, "", 280),
	}}
}

func TestChromiumAvatarGroup(t *testing.T) { avatarGroupChromium().chromium(t) }

func TestChromiumAvatarGroupRTL(t *testing.T) {
	s := avatarGroupChromium()
	s.name, s.rtl = "avatar_group_rtl", true
	s.cells = s.cells[:3]
	s.chromium(t)
}

// ---- Field slots and error content ---------------------------------------------------

// fieldMarkup is the markup of a vertical Field with a text input, built
// from the recipe's classes: the label (with an optional end slot), the
// control, the description (with an optional end slot) and the error.
func fieldMarkup(name string, label, labelEnd, desc, descEnd, errHTML string, in kit.Input, invalid bool) string {
	s := kit.DefaultStyle().Field
	id := "f-" + name
	attrs := inputAttrs(in)
	root := cn(shadcnFieldVertical, "text-destructive")
	rootAttrs := ""
	if invalid {
		attrs += ` aria-invalid="true"`
		rootAttrs = ` data-invalid="true"`
	} else {
		root = cn(shadcnFieldVertical)
	}
	control := fmt.Sprintf(`<input id="%s"%s class="%s">`, id, attrs, shadcnInput)
	lab := fmt.Sprintf(`<label data-slot="field-label" for="%s" class="%s">%s</label>`, id, cn(shadcnLabel, shadcnFieldLabel), label)
	if labelEnd != "" {
		lab = fmt.Sprintf(`<div class="%s">%s%s</div>`, s.LabelRow, lab, labelEnd)
	}
	d := ""
	if desc != "" {
		withError := ""
		if errHTML != "" {
			withError = s.DescriptionWithError
		}
		if descEnd != "" {
			d = fmt.Sprintf(`<div class="%s"><p data-slot="field-description" class="%s">%s</p>%s</div>`,
				cn(s.DescriptionRow, withError), cn(s.Description, s.DescriptionInRow), desc, descEnd)
		} else {
			d = fmt.Sprintf(`<p data-slot="field-description" class="%s">%s</p>`, cn(s.Description, withError), desc)
		}
	}
	e := ""
	if errHTML != "" {
		e = fmt.Sprintf(`<div role="alert" data-slot="field-error" class="%s">%s</div>`, shadcnFieldError, errHTML)
	}
	return fmt.Sprintf(`<div role="group" data-slot="field"%s data-orientation="vertical" data-probe="%s-field" class="%s">%s%s%s%s</div>`,
		rootAttrs, name, root, lab, control, d, e)
}

func fieldSlotsChromium() *sheet {
	forgot := buttonHTML(kit.Button{Variant: kit.Link, Size: kit.Sm, Class: "h-auto p-0 text-xs"}, "Forgot password?", "")
	muted := func(t string) string { return fmt.Sprintf(`<span class="text-xs text-muted-foreground">%s</span>`, t) }
	// The password bullets are Skia's in Chromium and Tela's here: the mean
	// of a cell with one is a little over the default.
	s := fieldSlotsSheet()
	s.mean = [2]float64{4.2, 4.2}
	return s.withMarkup(map[string]string{
		"label-end": fieldMarkup("label-end", "Password", forgot, "", "", "", kit.Input{Type: "password", DefaultValue: "hunter22"}, false),
		"rich-label": fieldMarkup("rich-label", `<span class="flex items-center gap-1">Email<span class="text-destructive">*</span></span>`, "", "", "", "",
			kit.Input{Type: "email", Placeholder: "m@example.com"}, false),
		"rich-description": fieldMarkup("rich-description", "Website", "",
			`<span>Your public page, as in <span class="underline underline-offset-4">the guidelines</span>.</span>`, "", "", kit.Input{Placeholder: "https://"}, false),
		"all-slots": fieldMarkup("all-slots", "Username", muted("Optional"), "Letters and digits only.", muted("3/20"), "", kit.Input{DefaultValue: "ana"}, false),
		"error": fieldMarkup("error", "Password", forgot, "At least 8 characters.", "", "Password is too short.",
			kit.Input{Type: "password", DefaultValue: "short"}, true),
	})
}

func TestChromiumFieldSlots(t *testing.T) { fieldSlotsChromium().chromium(t) }

func fieldErrorChromium() *sheet {
	return fieldErrorSheet().withMarkup(map[string]string{
		"plain": fieldMarkup("plain", "Username", "", "", "", "That name is taken.", kit.Input{DefaultValue: "ana"}, true),
		"content": fieldMarkup("content", "Username", "", "", "", `<span>That name is taken. <span class="underline underline-offset-4">See suggestions</span></span>`,
			kit.Input{DefaultValue: "ana"}, true),
		"list": fieldMarkup("list", "Password", "", "Pick a strong one.", "", `<div class="flex flex-col"><div>At least 8 characters</div><div>One digit</div></div>`,
			kit.Input{Type: "password", DefaultValue: "abc"}, true),
	})
}

func TestChromiumFieldError(t *testing.T) { fieldErrorChromium().chromium(t) }

// ---- Table sort states ---------------------------------------------------------------------

func tableSortChromium() *sheet {
	cols := []tcol{{header: "Invoice", width: "130px", sort: true}, {header: "Status", width: "130px", sort: true},
		{header: "Amount", width: "120px", right: true, sort: true}}
	rows := [][]string{{"INV001", "Paid", "$250.00"}, {"INV002", "Pending", "$150.00"}, {"INV003", "Paid", "$350.00"}}
	spec := func(name string, sort kit.SortState, edit func(*tableSpec)) cell {
		p := tableSpec{name: name, width: 380, cols: cols, rows: rows, fixed: true, sort: sort, sortStates: true, height: 150}
		if edit != nil {
			edit(&p)
		}
		return p.cell()
	}
	return &sheet{name: "table_sort", width: 900, edge: [2]float64{1, 1}, mean: [2]float64{6, 3.5}, cells: []cell{
		spec("default", kit.SortState{Column: "Invoice", Direction: kit.Ascending}, nil),
		spec("hide-unsorted", kit.SortState{Column: "Invoice", Direction: kit.Ascending}, func(p *tableSpec) { p.hideUnsorted = true }),
		spec("other", kit.SortState{Column: "Status", Direction: kit.Other}, nil),
		spec("variants", kit.SortState{Column: "Status", Direction: kit.Other}, func(p *tableSpec) {
			p.headClass = "aria-[sort=none]:text-muted-foreground aria-[sort=other]:text-amber-600 aria-[sort=ascending]:text-primary aria-[sort=descending]:text-primary"
		}),
	}}
}

func TestChromiumTableSort(t *testing.T) { tableSortChromium().chromium(t) }
