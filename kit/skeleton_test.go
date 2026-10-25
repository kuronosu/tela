package kit_test

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
)

// shadcn/ui's Skeleton (new-york-v4/ui/skeleton.tsx at kit.ShadcnCommit),
// verbatim.
const shadcnSkeleton = "animate-pulse rounded-md bg-accent"

func skeletonSheet() *sheet {
	sk := func(class string) string {
		return fmt.Sprintf(`<div class="%s"></div>`, cn(shadcnSkeleton, class))
	}
	return &sheet{
		name: "skeleton",
		// The pulse is a running animation: Chromium's screenshot would
		// catch it at some opacity, and Tela's is taken at its start.
		css: "*, *::before, *::after { animation: none !important; }",
		cells: []cell{
			{
				name: "profile",
				tela: h.Div(h.Class("flex items-center gap-4"),
					kit.Skeleton{Class: "size-12 rounded-full"},
					h.Div(h.Class("flex flex-col gap-2"), kit.Skeleton{Class: "h-4 w-[250px]"}, kit.Skeleton{Class: "h-4 w-[200px]"})),
				html: `<div class="flex items-center gap-4">` + sk("size-12 rounded-full") +
					`<div class="flex flex-col gap-2">` + sk("h-4 w-[250px]") + sk("h-4 w-[200px]") + `</div></div>`,
			},
			{
				name: "card", width: 240,
				tela: h.Div(h.Class("flex w-full flex-col gap-3"),
					kit.Skeleton{Class: "h-[125px] w-full rounded-xl"},
					h.Div(h.Class("flex flex-col gap-2"), kit.Skeleton{Class: "h-4 w-full"}, kit.Skeleton{Class: "h-4 w-4/5"})),
				html: `<div class="flex w-full flex-col gap-3">` + sk("h-[125px] w-full rounded-xl") +
					`<div class="flex flex-col gap-2">` + sk("h-4 w-full") + sk("h-4 w-4/5") + `</div></div>`,
			},
		},
	}
}

func TestSkeletonGolden(t *testing.T)   { skeletonSheet().golden(t) }
func TestChromiumSkeleton(t *testing.T) { skeletonSheet().chromium(t) }
