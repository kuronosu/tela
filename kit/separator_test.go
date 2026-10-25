package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Separator (new-york-v4/ui/separator.tsx at kit.ShadcnCommit),
// verbatim.
const shadcnSeparator = "shrink-0 bg-border data-[orientation=horizontal]:h-px data-[orientation=horizontal]:w-full data-[orientation=vertical]:h-full data-[orientation=vertical]:w-px"

func separatorSheet() *sheet {
	// A decorative separator is hidden from the semantics tree, which the
	// probes read, so the cells measure the boxes around them.
	return &sheet{name: "separator", cells: []cell{
		{
			name: "horizontal", width: 240,
			tela: h.Div(h.Class("flex w-full flex-col gap-2"),
				h.Div(h.Class("text-sm font-medium"), h.Text("Radix Primitives")),
				kit.Separator{},
				h.Div(h.Class("text-sm text-muted-foreground"), h.Text("An open-source UI component library."))),
			html: fmt.Sprintf(`<div class="flex w-full flex-col gap-2"><div class="text-sm font-medium">Radix Primitives</div><div data-orientation="horizontal" class="%s"></div><div class="text-sm text-muted-foreground">An open-source UI component library.</div></div>`, cn(shadcnSeparator)),
		},
		{
			name: "vertical",
			tela: h.Div(h.Class("flex h-5 items-center gap-4 text-sm"),
				h.Div(h.Text("Blog")), kit.Separator{Orientation: kit.Vertical},
				h.Div(h.Text("Docs")), kit.Separator{Orientation: kit.Vertical},
				h.Div(h.Text("Source"))),
			html: fmt.Sprintf(`<div class="flex h-5 items-center gap-4 text-sm"><div>Blog</div><div data-orientation="vertical" class="%[1]s"></div><div>Docs</div><div data-orientation="vertical" class="%[1]s"></div><div>Source</div></div>`, cn(shadcnSeparator)),
		},
		{
			name: "class", width: 200,
			tela: kit.Separator{Class: "bg-primary"},
			html: fmt.Sprintf(`<div data-orientation="horizontal" class="%s"></div>`, cn(shadcnSeparator, "bg-primary")),
		},
	}}
}

func TestSeparatorGolden(t *testing.T)   { separatorSheet().golden(t) }
func TestChromiumSeparator(t *testing.T) { separatorSheet().chromium(t) }
