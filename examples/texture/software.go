package main

import (
	"context"
	"image"
	"sync"

	"github.com/kuronosu/tela"
)

const softwareName = "the CPU, as pixels"

// software is the painter that draws with the CPU and submits pixels. Three
// images are enough: the one shown, the one that is ready, and the one it
// draws. Those of an old size are garbage as soon as Tela lets go of them.
type software struct {
	tex  *tela.Texture
	wake chan struct{} // Tela let go of one

	mu   sync.Mutex
	size image.Rectangle
	free []*image.RGBA // of size
	held int           // drawn, or Tela's
}

const softwareImages = 3

func newSoftware(tex *tela.Texture) *software {
	return &software{tex: tex, wake: make(chan struct{}, 1)}
}

func (s *software) about() string { return softwareName }

func (s *software) limit() int { return frameLimit }

// returned and trim: an image that comes back needs nothing of the loop.
func (s *software) returned() <-chan struct{} { return nil }

func (s *software) trim() {}

// close has nothing to free: the images Tela holds are garbage once it let
// go of them.
func (s *software) close() {}

func (s *software) paint(ctx context.Context, u *uniforms) error {
	img, err := s.image(ctx, image.Rect(0, 0, int(u.Size[0]), int(u.Size[1])))
	if err != nil {
		return err
	}
	drawScene(img, u)
	return s.tex.SubmitImage(img, func() { s.release(img) })
}

// image returns an image that Tela does not read.
func (s *software) image(ctx context.Context, size image.Rectangle) (*image.RGBA, error) {
	for {
		if img := s.take(size); img != nil {
			return img, nil
		}
		select {
		case <-s.wake:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
}

func (s *software) take(size image.Rectangle) *image.RGBA {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.size != size {
		s.size, s.free = size, nil
	}
	if s.held == softwareImages {
		return nil
	}
	s.held++
	if n := len(s.free); n > 0 {
		img := s.free[n-1]
		s.free = s.free[:n-1]
		return img
	}
	return image.NewRGBA(size)
}

// release takes back an image that Tela does not read any more. It runs on a
// goroutine of Tela's.
func (s *software) release(img *image.RGBA) {
	s.mu.Lock()
	s.held--
	if img.Rect == s.size {
		s.free = append(s.free, img)
	}
	s.mu.Unlock()
	select {
	case s.wake <- struct{}{}:
	default:
	}
}
