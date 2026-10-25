package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's buttonVariants (apps/v4/registry/new-york-v4/ui/button.tsx at
// kit.ShadcnCommit), verbatim.
const (
	shadcnButton = "inline-flex shrink-0 items-center justify-center gap-2 rounded-md text-sm font-medium whitespace-nowrap transition-all outline-none focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4"

	shadcnButtonDefault     = "bg-primary text-primary-foreground hover:bg-primary/90"
	shadcnButtonDestructive = "bg-destructive text-white hover:bg-destructive/90 focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40"
	shadcnButtonOutline     = "border bg-background shadow-xs hover:bg-accent hover:text-accent-foreground dark:border-input dark:bg-input/30 dark:hover:bg-input/50"
	shadcnButtonSecondary   = "bg-secondary text-secondary-foreground hover:bg-secondary/80"
	shadcnButtonGhost       = "hover:bg-accent hover:text-accent-foreground dark:hover:bg-accent/50"
	shadcnButtonLink        = "text-primary underline-offset-4 hover:underline"

	shadcnButtonSizeDefault = "h-9 px-4 py-2 has-[>svg]:px-3"
	shadcnButtonSizeXs      = "h-6 gap-1 rounded-md px-2 text-xs has-[>svg]:px-1.5 [&_svg:not([class*='size-'])]:size-3"
	shadcnButtonSizeSm      = "h-8 gap-1.5 rounded-md px-3 has-[>svg]:px-2.5"
	shadcnButtonSizeLg      = "h-10 rounded-md px-6 has-[>svg]:px-4"
	shadcnButtonSizeIcon    = "size-9"
	shadcnButtonSizeIconXs  = "size-6 rounded-md [&_svg:not([class*='size-'])]:size-3"
	shadcnButtonSizeIconSm  = "size-8"
	shadcnButtonSizeIconLg  = "size-10"
)

var shadcnButtonVariants = map[kit.Variant]string{
	kit.Default: shadcnButtonDefault, kit.Destructive: shadcnButtonDestructive, kit.Outline: shadcnButtonOutline,
	kit.Secondary: shadcnButtonSecondary, kit.Ghost: shadcnButtonGhost, kit.Link: shadcnButtonLink,
}

var shadcnButtonSizes = map[kit.Size]string{
	"": shadcnButtonSizeDefault, kit.Xs: shadcnButtonSizeXs, kit.Sm: shadcnButtonSizeSm, kit.Lg: shadcnButtonSizeLg,
	kit.Icon: shadcnButtonSizeIcon, kit.IconXs: shadcnButtonSizeIconXs, kit.IconSm: shadcnButtonSizeIconSm, kit.IconLg: shadcnButtonSizeIconLg,
}

// buttonHTML is the markup shadcn's Button renders for b, with inner as its
// content (text and icons); probe, when not empty, marks it as a probe.
func buttonHTML(b kit.Button, inner, probe string) string {
	attrs := ""
	if probe != "" {
		attrs += fmt.Sprintf(" data-probe=%q", probe)
	}
	if b.Disabled {
		attrs += " disabled"
	}
	return fmt.Sprintf(`<button%s class="%s">%s</button>`, attrs,
		cn(shadcnButton, shadcnButtonVariants[b.Variant], shadcnButtonSizes[b.Size], b.Class), inner)
}

// buttonCell is a button on both sides. inner is shadcn's content (text and
// icons) and the Tela side is b, whose Props mark the button as a probe.
func buttonCell(name string, b kit.Button, inner string) cell {
	b.Props = h.TestID(name + "-button")
	return cell{name: name, probes: []string{name + "-button"}, tela: b, html: buttonHTML(b, inner, name+"-button")}
}

var variantNames = []struct {
	v    kit.Variant
	name string
}{
	{kit.Default, "default"}, {kit.Secondary, "secondary"}, {kit.Outline, "outline"},
	{kit.Ghost, "ghost"}, {kit.Destructive, "destructive"}, {kit.Link, "link"},
}

var sizeNames = []struct {
	s    kit.Size
	name string
}{
	{kit.Xs, "xs"}, {kit.Sm, "sm"}, {"", "default"}, {kit.Lg, "lg"},
}

var iconSizeNames = []struct {
	s    kit.Size
	name string
}{
	{kit.IconXs, "icon-xs"}, {kit.IconSm, "icon-sm"}, {kit.Icon, "icon"}, {kit.IconLg, "icon-lg"},
}

func buttonSheet() *sheet {
	var cells []cell
	for _, v := range variantNames {
		cells = append(cells, buttonCell("variant-"+v.name, kit.Button{Variant: v.v, Children: h.Text("Button")}, "Button"))
	}
	for _, sz := range sizeNames {
		cells = append(cells, buttonCell("size-"+sz.name, kit.Button{Variant: kit.Outline, Size: sz.s, Children: h.Text("Button")}, "Button"))
	}
	for _, sz := range iconSizeNames {
		cells = append(cells, buttonCell("size-"+sz.name, kit.Button{Variant: kit.Outline, Size: sz.s, Icon: iconChevronRight, AriaLabel: "Next"},
			svgHTML(iconChevronRight)))
	}
	cells = append(cells,
		buttonCell("icon-start", kit.Button{Icon: iconMail, Children: h.Text("Login with email")}, svgHTML(iconMail)+"Login with email"),
		buttonCell("icon-end", kit.Button{Variant: kit.Secondary, IconEnd: iconArrowRight, Children: h.Text("Continue")}, "Continue"+svgHTML(iconArrowRight)),
		buttonCell("icon-sm", kit.Button{Variant: kit.Outline, Size: kit.Sm, Icon: iconMail, Children: h.Text("Email")}, svgHTML(iconMail)+"Email"),
		buttonCell("icon-lg", kit.Button{Variant: kit.Outline, Size: kit.Lg, IconEnd: iconArrowRight, Children: h.Text("Next")}, "Next"+svgHTML(iconArrowRight)),
		buttonCell("icon-xs", kit.Button{Variant: kit.Secondary, Size: kit.Xs, Icon: iconCheck, Children: h.Text("Done")}, svgHTML(iconCheck)+"Done"),
		buttonCell("icon-destructive", kit.Button{Variant: kit.Destructive, Icon: iconCircleAlert, Children: h.Text("Delete")}, svgHTML(iconCircleAlert)+"Delete"),
		buttonCell("disabled-default", kit.Button{Disabled: true, Children: h.Text("Disabled")}, "Disabled"),
		buttonCell("disabled-outline", kit.Button{Variant: kit.Outline, Disabled: true, Children: h.Text("Disabled")}, "Disabled"),
		buttonCell("disabled-ghost", kit.Button{Variant: kit.Ghost, Disabled: true, Children: h.Text("Disabled")}, "Disabled"),
	)
	return &sheet{name: "button", cells: cells, slot: 176}
}

func TestButtonGolden(t *testing.T)   { buttonSheet().golden(t) }
func TestChromiumButton(t *testing.T) { buttonSheet().chromium(t) }

var _ tela.Component = kit.Button{}
