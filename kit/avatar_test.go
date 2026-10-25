package kit_test

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"testing"

	"github.com/kuronosu/tela/h"
	"github.com/kuronosu/tela/kit"
	"github.com/kuronosu/tela/telatest"
)

func TestAvatarFallbackClass(t *testing.T) {
	app := telatest.Mount(t, kit.Avatar{Fallback: "MT", FallbackClass: "bg-linear-to-br from-sky-400 to-indigo-500 text-white"})
	app.ExpectCount(telatest.ByClass("from-sky-400", "to-indigo-500", "size-full"), 1)
	app.ExpectCount(telatest.ByClass("bg-muted"), 1) // the recipe's classes stay under it
}

// shadcn/ui's Avatar parts (new-york-v4/ui/avatar.tsx at kit.ShadcnCommit),
// verbatim.
const (
	shadcnAvatar         = "group/avatar relative flex size-8 shrink-0 overflow-hidden rounded-full select-none data-[size=lg]:size-10 data-[size=sm]:size-6"
	shadcnAvatarImage    = "aspect-square size-full"
	shadcnAvatarFallback = "flex size-full items-center justify-center rounded-full bg-muted text-sm text-muted-foreground group-data-[size=sm]/avatar:text-xs"
)

// portrait is a 64 x 64 picture with flat areas and a gradient, so that the
// two renderers' resampling of it differs little.
func portrait() image.Image {
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for y := range 64 {
		for x := range 64 {
			c := color.RGBA{R: 0x3b, G: 0x82, B: 0xf6, A: 255} // blue-500
			switch {
			case (x-32)*(x-32)+(y-24)*(y-24) < 100: // a head
				c = color.RGBA{R: 0xfd, G: 0xe6, B: 0x8a, A: 255}
			case y > 44: // shoulders
				c = color.RGBA{R: 0x1e, G: 0x29, B: 0x3b, A: 255}
			case y < 14:
				c = color.RGBA{R: uint8(0x60 + x), G: 0xa5, B: 0xfa, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func portraitURL() string {
	var buf bytes.Buffer
	if err := png.Encode(&buf, portrait()); err != nil {
		panic(err)
	}
	return "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
}

var avatarSizes = []struct {
	s    kit.Size
	name string
	attr string
}{
	{kit.Sm, "sm", "sm"}, {"", "default", "default"}, {kit.Lg, "lg", "lg"},
}

func avatarCell(name string, a kit.Avatar, withImage bool, attr string) cell {
	a.Props = h.TestID(name + "-avatar")
	inner := ""
	if withImage {
		a.Src = h.Src(portrait())
		a.Alt = "Portrait"
		inner = fmt.Sprintf(`<img class="%s" alt="Portrait" src="%s">`, shadcnAvatarImage, portraitURL())
	} else {
		inner = fmt.Sprintf(`<span class="%s">%s</span>`, shadcnAvatarFallback, a.Fallback)
	}
	return cell{
		name: name, probes: []string{name + "-avatar"}, tela: a,
		html: fmt.Sprintf(`<span data-probe="%s-avatar" data-size="%s" class="%s">%s</span>`, name, attr, cn(shadcnAvatar, a.Class), inner),
	}
}

func avatarSheet() *sheet {
	var cells []cell
	for _, sz := range avatarSizes {
		cells = append(cells, avatarCell("fallback-"+sz.name, kit.Avatar{Size: sz.s, Fallback: "CN"}, false, sz.attr))
	}
	for _, sz := range avatarSizes {
		cells = append(cells, avatarCell("image-"+sz.name, kit.Avatar{Size: sz.s, Fallback: "CN"}, true, sz.attr))
	}
	cells = append(cells, avatarCell("fallback-class", kit.Avatar{Fallback: "MT", Class: "size-12"}, false, "default"))
	return &sheet{name: "avatar", cells: cells, slot: 64}
}

func TestAvatarGolden(t *testing.T)   { avatarSheet().golden(t) }
func TestChromiumAvatar(t *testing.T) { avatarSheet().chromium(t) }
