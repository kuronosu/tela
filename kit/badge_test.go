package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's badgeVariants (new-york-v4/ui/badge.tsx at kit.ShadcnCommit),
// verbatim.
const shadcnBadge = "inline-flex w-fit shrink-0 items-center justify-center gap-1 overflow-hidden rounded-full border border-transparent px-2 py-0.5 text-xs font-medium whitespace-nowrap transition-[color,box-shadow] focus-visible:border-ring focus-visible:ring-[3px] focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 dark:aria-invalid:ring-destructive/40 [&>svg]:pointer-events-none [&>svg]:size-3"

var shadcnBadgeVariants = map[kit.Variant]string{
	kit.Default:     "bg-primary text-primary-foreground [a&]:hover:bg-primary/90",
	kit.Secondary:   "bg-secondary text-secondary-foreground [a&]:hover:bg-secondary/90",
	kit.Destructive: "bg-destructive text-white focus-visible:ring-destructive/20 dark:bg-destructive/60 dark:focus-visible:ring-destructive/40 [a&]:hover:bg-destructive/90",
	kit.Outline:     "border-border text-foreground [a&]:hover:bg-accent [a&]:hover:text-accent-foreground",
	kit.Ghost:       "[a&]:hover:bg-accent [a&]:hover:text-accent-foreground",
	kit.Link:        "text-primary underline-offset-4 [a&]:hover:underline",
}

func badgeCell(name string, b kit.Badge, inner string) cell {
	b.Props = h.TestID(name + "-badge")
	return cell{
		name: name, probes: []string{name + "-badge"}, tela: b,
		html: fmt.Sprintf(`<span data-probe="%s-badge" class="%s">%s</span>`, name,
			cn(shadcnBadge, shadcnBadgeVariants[b.Variant], b.Class), inner),
	}
}

func badgeSheet() *sheet {
	var cells []cell
	for _, v := range variantNames {
		cells = append(cells, badgeCell("variant-"+v.name, kit.Badge{Variant: v.v, Children: h.Text("Badge")}, "Badge"))
	}
	cells = append(cells,
		badgeCell("icon-check", kit.Badge{Variant: kit.Secondary, Icon: iconCheck, Children: h.Text("Verified")}, svgHTML(iconCheck)+"Verified"),
		badgeCell("icon-alert", kit.Badge{Variant: kit.Destructive, Icon: iconCircleAlert, Children: h.Text("Failed")}, svgHTML(iconCircleAlert)+"Failed"),
		badgeCell("count", kit.Badge{Class: "h-5 min-w-5 rounded-full px-1 tabular-nums", Children: h.Text("8")}, "8"),
		badgeCell("count-wide", kit.Badge{Variant: kit.Outline, Class: "h-5 min-w-5 rounded-full px-1 tabular-nums", Children: h.Text("99+")}, "99+"),
	)
	return &sheet{name: "badge", cells: cells, slot: 96}
}

func TestBadgeGolden(t *testing.T)   { badgeSheet().golden(t) }
func TestChromiumBadge(t *testing.T) { badgeSheet().chromium(t) }
