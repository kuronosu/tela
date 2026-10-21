package main

import (
	"fmt"
	"testing"

	"github.com/kuronosu/tela/telatest"
)

func mount(t *testing.T, opts ...telatest.Option) *telatest.App {
	t.Helper()
	opts = append([]telatest.Option{telatest.Size(800, 450)}, opts...)
	return telatest.MountFunc(t, Hello, opts...)
}

func TestText(t *testing.T) {
	app := mount(t)
	for _, s := range []string{"Hello, Tela.", "Interfaces, woven with Go."} {
		if app.Count(telatest.ByText(s, telatest.Exact())) != 1 {
			t.Errorf("text %q not found once", s)
		}
	}
}

// TestGolden is the screen at scale 1 and 2; the x2 one is docs/images/hello.png.
func TestGolden(t *testing.T) {
	for _, scale := range []float64{1, 2} {
		t.Run(fmt.Sprintf("x%g", scale), func(t *testing.T) {
			mount(t, telatest.Scale(scale)).Golden(fmt.Sprintf("hello_x%g.png", scale))
		})
	}
}
