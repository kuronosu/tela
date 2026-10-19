package anim

import (
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

func TestBlendClipPath(t *testing.T) {
	var a, b, c, dst style.ComputedStyle
	a.ClipPath = style.ClipPath{Shape: style.ClipCircle, Values: []style.LP{style.PctLP(10), style.PctLP(50), style.PctLP(50)}}
	b.ClipPath = style.ClipPath{Shape: style.ClipCircle, Values: []style.LP{style.PctLP(70), style.PxLP(20), style.PctLP(50)}}
	c.ClipPath = style.ClipPath{Shape: style.ClipInset, Values: make([]style.LP, 12)}
	if !CanInterpolate(PropClipPath, &a, &b) || CanInterpolate(PropClipPath, &a, &c) || Equal(PropClipPath, &a, &b) {
		t.Fatal("CanInterpolate / Equal")
	}
	Blend(PropClipPath, &dst, &a, &b, 0.5)
	want := []style.LP{style.PctLP(40), {Px: 10, Pct: 25}, style.PctLP(50)}
	if dst.ClipPath.Shape != style.ClipCircle || len(dst.ClipPath.Values) != 3 {
		t.Fatalf("half way: %+v", dst.ClipPath)
	}
	for i := range want {
		if dst.ClipPath.Values[i] != want[i] {
			t.Errorf("half way: %+v", dst.ClipPath)
		}
	}
	if &dst.ClipPath.Values[0] == &b.ClipPath.Values[0] {
		t.Error("the values alias the end style's")
	}
	Blend(PropClipPath, &dst, &a, &c, 0.4)
	if !dst.ClipPath.Equal(&a.ClipPath) {
		t.Errorf("discrete before 0.5: %+v", dst.ClipPath)
	}
	Blend(PropClipPath, &dst, &a, &c, 0.6)
	if !dst.ClipPath.Equal(&c.ClipPath) || PropClipPath.Class() != ClassInk || PropClipPath.String() != "clip-path" {
		t.Errorf("discrete after 0.5: %+v", dst.ClipPath)
	}
}
