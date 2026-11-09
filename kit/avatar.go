// Portions of this file follow shadcn/ui, avatar.tsx (apps/v4/registry/new-york-v4/ui) at the commit
// kit.ShadcnCommit (https://github.com/shadcn-ui/ui), Copyright (c) 2023 shadcn,
// licensed under the MIT License.
// See THIRD_PARTY.md.

package kit

import (
	"hash/fnv"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/h"
)

// AvatarColor is the background of an [Avatar]'s fallback among the ones its
// recipe defines. Like [Variant] it is open: an app adds a color by naming
// it in the recipe's Colors map.
type AvatarColor string

// AvatarAuto picks the color from the avatar's name: the same name always
// gets the same color.
const AvatarAuto AvatarColor = "auto"

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
	// Colors are the fallback's backgrounds (with their text color), by
	// name. They replace the muted disc.
	Colors map[AvatarColor]string
	// AutoColors are the names of Colors that [AvatarAuto] chooses among.
	AutoColors []AvatarColor
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
		Colors: map[AvatarColor]string{
			"red":     "bg-red-600 text-white",
			"orange":  "bg-orange-700 text-white",
			"amber":   "bg-amber-700 text-white",
			"emerald": "bg-emerald-700 text-white",
			"teal":    "bg-teal-700 text-white",
			"sky":     "bg-sky-700 text-white",
			"indigo":  "bg-indigo-600 text-white",
			"violet":  "bg-violet-600 text-white",
			"pink":    "bg-pink-600 text-white",
		},
		AutoColors: []AvatarColor{"red", "orange", "amber", "emerald", "teal", "sky", "indigo", "violet", "pink"},
	}
}

// Avatar is a round picture of a person or an account, shadcn/ui's Avatar,
// with the initials to show while there is no picture. The picture is
// painted over the fallback, so when Src is not given or the image does not
// draw, the fallback is what shows.
//
//	kit.Avatar{Src: h.SrcPath("faces/mei.png"), Alt: "Mei Tanaka", Fallback: "MT"}
//	kit.Avatar{Size: kit.Lg, Fallback: "AB"}
//
// Name stands for the person: it is the Alt of the picture, the initials of
// the fallback when there is no Fallback text, and with Color [AvatarAuto]
// the seed of the fallback's color, so a list of people has a color each
// that stays the same everywhere they appear:
//
//	kit.Avatar{Name: "Mei Tanaka", Color: kit.AvatarAuto}  // "MT" on a color chosen by the name
//	kit.Avatar{Name: "Ada Lovelace", Color: "indigo"}      // a color of the recipe, by name
type Avatar struct {
	// Size is Default (32 px), Sm (24 px) or Lg (40 px).
	Size Size
	// Src is the picture: h.Src, h.SrcPath, h.SrcBytes or h.SrcFS. Nil
	// shows the fallback only.
	Src tela.Child
	// Alt names the picture for assistive technologies; it defaults to Name.
	Alt string
	// Name is the person's name. Without a Fallback, the first letters of
	// its first and last words ("Mei Tanaka" is "MT") are the fallback.
	Name string
	// Fallback is the text shown without a picture, normally initials.
	Fallback string
	// Color is the fallback's background: [AvatarAuto] to choose it from
	// the Name (or the Fallback without one), or a name from the recipe's
	// Colors ("red", "orange", "amber", "emerald", "teal", "sky", "indigo",
	// "violet", "pink"). The zero value is the muted disc.
	Color AvatarColor
	// Class is added after the recipe's Root classes; FallbackClass does the
	// same for the fallback disc, such as a gradient of its own per person.
	Class         string
	FallbackClass string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the avatar element.
func (a Avatar) Build() tela.Node {
	s := &UseStyle().Avatar
	kids := []tela.Child{cls(s.Root, pick("Avatar", "Size", s.Sizes, a.Size), a.Class), a.Props}
	text := a.Fallback
	if text == "" {
		text = Initials(a.Name)
	}
	if text != "" {
		kids = append(kids, h.Div(cls(s.Fallback, s.FallbackSizes[a.Size], a.colorClass(s), a.FallbackClass), h.Text(text)))
	}
	if a.Src != nil {
		img := []tela.Child{cls(s.Image), a.Src}
		alt := a.Alt
		if alt == "" {
			alt = a.Name
		}
		if alt != "" {
			img = append(img, h.Alt(alt))
		}
		kids = append(kids, h.Img(img...))
	}
	return h.Div(kids...)
}

// colorClass returns the classes of the fallback's Color.
func (a Avatar) colorClass(s *AvatarStyle) string {
	switch a.Color {
	case "":
		return ""
	case AvatarAuto:
		seed := a.Name
		if seed == "" {
			seed = a.Fallback
		}
		if seed == "" || len(s.AutoColors) == 0 {
			return ""
		}
		f := fnv.New32a()
		f.Write([]byte(strings.ToLower(strings.TrimSpace(seed))))
		return s.Colors[s.AutoColors[int(f.Sum32()%uint32(len(s.AutoColors)))]]
	}
	return pick("Avatar", "Color", s.Colors, a.Color)
}

// Initials returns the first letters of the first and the last word of a
// name, in upper case: "Mei Tanaka" is "MT", "Ada" is "A" and "" is "". It
// is what an [Avatar] with a Name and no Fallback shows.
func Initials(name string) string {
	words := strings.Fields(name)
	if len(words) == 0 {
		return ""
	}
	first := func(w string) string {
		r, _ := utf8.DecodeRuneInString(w)
		return string(unicode.ToUpper(r))
	}
	if len(words) == 1 {
		return first(words[0])
	}
	return first(words[0]) + first(words[len(words)-1])
}

// AvatarGroupStyle is the recipe of [AvatarGroup] (shadcn/ui's avatar group
// example: a row of avatars that overlap, each with a ring in the
// background color).
type AvatarGroupStyle struct {
	// Root overlaps the avatars: the next one covers the end of the
	// previous.
	Root string
	// Avatar is added to every avatar of the group, including the overflow
	// count: a ring in the background color that separates them.
	Avatar string
	// Overflow is added to the "+N" avatar after Avatar.
	Overflow string
}

func init() {
	defaultStyle.AvatarGroup = AvatarGroupStyle{
		Root:     "flex -space-x-2",
		Avatar:   "ring-2 ring-background",
		Overflow: "",
	}
}

// AvatarGroup is a row of overlapping avatars with a count of the ones that
// do not fit, as shadcn/ui's documentation builds with `-space-x-2` and a
// ring in the background color:
//
//	kit.AvatarGroup{Max: 3, Avatars: []kit.Avatar{
//	    {Name: "Mei Tanaka", Color: kit.AvatarAuto},
//	    {Name: "Ada Lovelace", Color: kit.AvatarAuto},
//	    {Name: "Alan Turing", Color: kit.AvatarAuto},
//	    {Name: "Grace Hopper", Color: kit.AvatarAuto},
//	    {Name: "Linus Torvalds", Color: kit.AvatarAuto},
//	}}  // three avatars and "+2"
//
// The ring is as wide as the page's background (ring-background), so the
// avatars are separated on any surface that uses it; on a card or a
// different surface set the ring with Class ("ring-card"). Later avatars
// cover earlier ones, and the overlap follows the direction of the page.
type AvatarGroup struct {
	// Avatars are the people, in order.
	Avatars []Avatar
	// Max is the most avatars drawn; the others are counted in a "+N"
	// avatar at the end. 0 draws all of them.
	Max int
	// Total is how many people there are when more than Avatars lists (the
	// rest was not loaded): the "+N" counts Total minus the ones drawn.
	Total int
	// Size is the size of the avatars that have none of their own, and of
	// the "+N" one.
	Size Size
	// AriaLabel names the group for assistive technologies ("Team
	// members"); without it the group has no name.
	AriaLabel string
	// Class is added after the recipe's Root classes. AvatarClass is added
	// to every avatar after the recipe's ring (including the "+N"):
	// AvatarClass: "ring-card".
	Class       string
	AvatarClass string
	// Props are more props for the root element.
	Props tela.Child
}

// Build returns the group element.
func (g AvatarGroup) Build() tela.Node {
	s := &UseStyle().AvatarGroup
	shown := len(g.Avatars)
	if g.Max > 0 && g.Max < shown {
		shown = g.Max
	}
	kids := []tela.Child{cls(s.Root, g.Class), h.Role("group"), g.Props}
	if g.AriaLabel != "" {
		kids = append(kids, h.AriaLabel(g.AriaLabel))
	}
	for _, a := range g.Avatars[:shown] {
		if a.Size == "" {
			a.Size = g.Size
		}
		a.Class = joinClass(s.Avatar, g.AvatarClass, a.Class)
		kids = append(kids, a)
	}
	if more := max(g.Total, len(g.Avatars)) - shown; more > 0 {
		kids = append(kids, Avatar{Size: g.Size, Fallback: "+" + strconv.Itoa(more), Props: h.Dir("ltr"), Class: joinClass(s.Avatar, s.Overflow, g.AvatarClass)})
	}
	return h.Div(kids...)
}
