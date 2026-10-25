package kit_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Card parts (new-york-v4/ui/card.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnCard            = "flex flex-col gap-6 rounded-xl border bg-card py-6 text-card-foreground shadow-sm"
	shadcnCardHeader      = "@container/card-header grid auto-rows-min grid-rows-[auto_auto] items-start gap-2 px-6 has-data-[slot=card-action]:grid-cols-[1fr_auto] [.border-b]:pb-6"
	shadcnCardTitle       = "leading-none font-semibold"
	shadcnCardDescription = "text-sm text-muted-foreground"
	shadcnCardAction      = "col-start-2 row-span-2 row-start-1 self-start justify-self-end"
	shadcnCardContent     = "px-6"
	shadcnCardFooter      = "flex items-center px-6 [.border-t]:pt-6"
)

// part is a piece of a cell on both sides: a component and its markup.
type part struct {
	c    tela.Component
	html string
}

// btn is a kit.Button and shadcn's markup for it.
func btn(b kit.Button, label string) part {
	return part{c: b, html: buttonHTML(b, label, "")}
}

// parts joins parts into one component and one markup string.
func parts(ps ...part) part {
	kids := make([]tela.Child, len(ps))
	var sb strings.Builder
	for i, p := range ps {
		kids[i] = p.c
		sb.WriteString(p.html)
	}
	return part{c: h.Fragment(kids...), html: sb.String()}
}

// cardSpec describes a card cell.
type cardSpec struct {
	title, description, content string
	action, footer              *part
	// extra classes for the root, and for the header and footer, which
	// shadcn's [.border-b]:pb-6 and [.border-t]:pt-6 follow.
	class, headerClass, footerClass string
}

func cardCell(name string, p cardSpec) cell {
	p.class = strings.TrimSpace("w-full " + p.class)
	c := kit.Card{Props: h.TestID(name + "-card"), Class: p.class, HeaderClass: p.headerClass, FooterClass: p.footerClass}
	var body, header string
	if p.title != "" {
		c.Title = h.Text(p.title)
		header += fmt.Sprintf(`<div class="%s">%s</div>`, shadcnCardTitle, p.title)
	}
	if p.description != "" {
		c.Description = h.Text(p.description)
		header += fmt.Sprintf(`<div class="%s">%s</div>`, shadcnCardDescription, p.description)
	}
	if p.action != nil {
		c.Action = p.action.c
		header += fmt.Sprintf(`<div data-slot="card-action" class="%s">%s</div>`, shadcnCardAction, p.action.html)
	}
	if header != "" {
		body += fmt.Sprintf(`<div data-slot="card-header" class="%s">%s</div>`, shadcnCardHeader, header)
	}
	if p.content != "" {
		c.Children = h.P(h.Text(p.content), h.TestID(name+"-content"))
		body += fmt.Sprintf(`<div class="%s"><p data-probe="%s">%s</p></div>`, shadcnCardContent, name+"-content", p.content)
	}
	if p.footer != nil {
		c.Footer = p.footer.c
		body += fmt.Sprintf(`<div class="%s">%s</div>`, shadcnCardFooter, p.footer.html)
	}
	return cell{
		name: name, width: 360, probes: []string{name + "-card", name + "-content"}, tela: c,
		html: fmt.Sprintf(`<div data-probe="%s" class="%s">%s</div>`, name+"-card", cn(shadcnCard, p.class), body),
	}
}

func cardSheet() *sheet {
	footer := parts(btn(kit.Button{Variant: kit.Outline, Children: h.Text("Cancel")}, "Cancel"), btn(kit.Button{Children: h.Text("Deploy")}, "Deploy"))
	footer.html = `<div class="flex w-full justify-end gap-2">` + footer.html + `</div>`
	footer.c = h.Div(h.Class("flex w-full justify-end gap-2"), footer.c)
	signup := btn(kit.Button{Variant: kit.Link, Children: h.Text("Sign Up")}, "Sign Up")
	closeBtn := btn(kit.Button{Variant: kit.Ghost, Size: kit.IconSm, Icon: iconX, AriaLabel: "Close"}, svgHTML(iconX))
	full := btn(kit.Button{Class: "w-full", Children: h.Text("Login")}, "Login")
	cells := []cell{
		cardCell("full", cardSpec{title: "Create project", description: "Deploy your new project in one click.",
			content: "Your project will be live in a few seconds.", footer: &footer}),
		cardCell("action", cardSpec{title: "Login to your account", description: "Enter your email below to login.",
			content: "The form goes here.", action: &signup, footer: &full}),
		cardCell("close", cardSpec{title: "Settings", description: "Manage your preferences.", action: &closeBtn,
			content: "Choose what to be notified about."}),
		cardCell("title-only", cardSpec{title: "Notifications"}),
		cardCell("content-only", cardSpec{content: "A card with only content."}),
	}
	return &sheet{name: "card", cells: cells}
}

func TestCardGolden(t *testing.T)   { cardSheet().golden(t) }
func TestChromiumCard(t *testing.T) { cardSheet().chromium(t) }
