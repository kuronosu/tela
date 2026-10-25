package kit

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// AvatarStyle is the recipe of [Avatar] (shadcn/ui's Avatar, AvatarImage and
// AvatarFallback).
type AvatarStyle struct {
	Root string
	// Sizes are the diameters: the default (32 px), Sm and Lg.
	Sizes map[Size]string
	// Image is the picture, laid over the fallback.
	Image string
	// Fallback is the initials on a muted disc, drawn under the image.
	Fallback string
	// FallbackSizes are the fallback's classes per avatar size (a smaller
	// text for Sm); a size the map does not define adds none.
	FallbackSizes map[Size]string
}

func init() {
	defaultStyle.Avatar = AvatarStyle{
		Root: "relative flex size-8 shrink-0 overflow-hidden rounded-full select-none",
		Sizes: map[Size]string{
			"": "",
			Sm: "size-6",
			Lg: "size-10",
		},
		Image:    "absolute inset-0 aspect-square size-full object-cover",
		Fallback: "flex size-full items-center justify-center rounded-full bg-muted text-sm text-muted-foreground",
		FallbackSizes: map[Size]string{
			Sm: "text-xs",
		},
	}
}

// Avatar is a round picture of a person or an account, shadcn/ui's Avatar,
// with the initials to show while there is no picture. The picture is
// painted over the fallback, so when Src is not given or the image does not
// draw, the fallback is what shows.
//
//	kit.Avatar{Src: h.SrcPath("faces/mei.png"), Alt: "Mei Tanaka", Fallback: "MT"}
//	kit.Avatar{Size: kit.Lg, Fallback: "AB"}
type Avatar struct {
	// Size is Default (32 px), Sm (24 px) or Lg (40 px).
	Size Size
	// Src is the picture: h.Src, h.SrcPath, h.SrcBytes or h.SrcFS. Nil
	// shows the fallback only.
	Src tela.Child
	// Alt names the picture for assistive technologies.
	Alt string
	// Fallback is the text shown without a picture, normally initials.
	Fallback string
	// Class is added after the recipe's Root classes.
	Class string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the avatar element.
func (a Avatar) Build() tela.Node {
	s := &UseStyle().Avatar
	kids := []tela.Child{cls(s.Root, pick("Avatar", "Size", s.Sizes, a.Size), a.Class), a.Props}
	if a.Fallback != "" {
		kids = append(kids, h.Div(cls(s.Fallback, s.FallbackSizes[a.Size]), h.Text(a.Fallback)))
	}
	if a.Src != nil {
		img := []tela.Child{cls(s.Image), a.Src}
		if a.Alt != "" {
			img = append(img, h.Alt(a.Alt))
		}
		kids = append(kids, h.Img(img...))
	}
	return h.Div(kids...)
}
