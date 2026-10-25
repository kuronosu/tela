package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// SkeletonStyle is the recipe of [Skeleton] (shadcn/ui's Skeleton).
type SkeletonStyle struct {
	Root string
}

func init() {
	defaultStyle.Skeleton = SkeletonStyle{Root: "animate-pulse rounded-md bg-accent"}
}

// Skeleton is a placeholder that pulses while content loads, shadcn/ui's
// Skeleton. Size it with Class, to the shape of what it stands for.
//
//	h.Div(h.Class("flex items-center gap-4"),
//	    kit.Skeleton{Class: "size-12 rounded-full"},
//	    h.Div(h.Class("flex flex-col gap-2"),
//	        kit.Skeleton{Class: "h-4 w-[250px]"},
//	        kit.Skeleton{Class: "h-4 w-[200px]"}))
//
// Mark the region that loads with h.AriaBusy(true) so assistive technologies
// know it is not final.
type Skeleton struct {
	// Class is added after the recipe's classes.
	Class string
	// Props are more props for the element.
	Props tela.Child
}

// Build returns the skeleton element.
func (s Skeleton) Build() tela.Node {
	r := &UseStyle().Skeleton
	return h.Div(h.Class(r.Root, s.Class), s.Props)
}
