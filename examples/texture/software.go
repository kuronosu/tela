package main

import (
	"context"
	"image"

	"github.com/kuronosu/tela"
)

const softwareName = "the CPU, as pixels"

// software is the painter that draws with the CPU and submits pixels. Three
// images are enough: the one shown, the one that is ready, and the one it
// draws.
type software struct {
	tex  *tela.Texture
	free chan *image.RGBA
	made int
}

const softwareImages = 3

func newSoftware(tex *tela.Texture) *software {
	return &software{tex: tex, free: make(chan *image.RGBA, softwareImages)}
}

func (s *software) about() string { return softwareName }

// close has nothing to free: the images Tela holds are garbage once it let
// go of them.
func (s *software) close() {}

func (s *software) paint(ctx context.Context, u *uniforms) error {
	img, err := s.image(ctx, int(u.Size[0]), int(u.Size[1]))
	if err != nil {
		return err
	}
	drawScene(img, u)
	return s.tex.SubmitImage(img, func() { s.free <- img }) // it has room for all
}

// image returns an image that Tela does not read.
func (s *software) image(ctx context.Context, w, h int) (*image.RGBA, error) {
	var img *image.RGBA
	select {
	case img = <-s.free:
	default:
		if s.made < softwareImages {
			s.made++
			break
		}
		select {
		case img = <-s.free:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if r := image.Rect(0, 0, w, h); img == nil || img.Rect != r {
		img = image.NewRGBA(r) // the first, or of another size
	}
	return img, nil
}
