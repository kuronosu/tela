package anim

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/kuronosu/tela/internal/style"
)

// TestBuiltinKeyframesMatchTailwindCSS compares the Go keyframes with the
// @keyframes rules of the vendored Tailwind theme, so that updating the
// vendored CSS shows up here.
func TestBuiltinKeyframesMatchTailwindCSS(t *testing.T) {
	data, err := os.ReadFile("../style/gen/tailwind/theme.css")
	if err != nil {
		t.Fatal(err)
	}
	css := string(data)
	rule := regexp.MustCompile(`@keyframes\s+([\w-]+)\s*\{`)
	found := map[string]bool{}
	for _, m := range rule.FindAllStringSubmatchIndex(css, -1) {
		name := css[m[2]:m[3]]
		body := block(css[m[1]:])
		want := parseKeyframes(t, name, body)
		got, ok := Builtin(name)
		if !ok {
			t.Errorf("@keyframes %s in the Tailwind theme has no built-in", name)
			continue
		}
		found[name] = true
		if len(got.Frames) != len(want.Frames) {
			t.Errorf("%s: %d keyframes, the CSS has %d", name, len(got.Frames), len(want.Frames))
			continue
		}
		for i := range want.Frames {
			g, w := got.Frames[i], want.Frames[i]
			if g.Offset != w.Offset || (g.Opacity == nil) != (w.Opacity == nil) || (g.Transform == nil) != (w.Transform == nil) ||
				(g.Opacity != nil && *g.Opacity != *w.Opacity) || (g.Transform != nil && *g.Transform != *w.Transform) || g.Easing != w.Easing {
				t.Errorf("%s keyframe %d: Go %+v, CSS %+v", name, i, g, w)
			}
		}
	}
	for name := range builtinKeyframes {
		if !found[name] {
			t.Errorf("built-in %q is not in the Tailwind theme", name)
		}
	}
}

// block returns the text up to the brace that closes an opened block.
func block(s string) string {
	depth := 1
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			if depth--; depth == 0 {
				return s[:i]
			}
		}
	}
	return s
}

var frameRule = regexp.MustCompile(`([\w%.,\s]+)\{([^}]*)\}`)

// parseKeyframes reads the keyframes of a @keyframes body: offsets (also
// from, to and lists), opacity, transform (rotate, scale, translateY, none)
// and animation-timing-function.
func parseKeyframes(t *testing.T, name, body string) *Keyframes {
	t.Helper()
	k := &Keyframes{Name: name}
	for _, m := range frameRule.FindAllStringSubmatch(body, -1) {
		var offsets []float64
		for _, o := range strings.Split(m[1], ",") {
			switch o = strings.TrimSpace(o); o {
			case "from":
				offsets = append(offsets, 0)
			case "to":
				offsets = append(offsets, 1)
			default:
				v, err := strconv.ParseFloat(strings.TrimSuffix(o, "%"), 64)
				if err != nil {
					t.Fatalf("%s: offset %q", name, o)
				}
				offsets = append(offsets, v/100)
			}
		}
		for _, off := range offsets {
			f := Keyframe{Offset: off}
			for _, decl := range strings.Split(m[2], ";") {
				prop, val, ok := strings.Cut(decl, ":")
				if !ok {
					continue
				}
				prop, val = strings.TrimSpace(prop), strings.TrimSpace(val)
				switch prop {
				case "opacity":
					v, err := strconv.ParseFloat(val, 32)
					if err != nil {
						t.Fatalf("%s: opacity %q", name, val)
					}
					o := float32(v)
					f.Opacity = &o
				case "transform":
					tr := IdentityTransform()
					if val != "none" {
						fn, arg, _ := strings.Cut(strings.TrimSuffix(val, ")"), "(")
						n, err := strconv.ParseFloat(strings.TrimRight(arg, "deg%"), 32)
						if err != nil {
							t.Fatalf("%s: transform %q", name, val)
						}
						switch fn {
						case "rotate":
							tr.Rotate = float32(n)
						case "scale":
							tr.ScaleX, tr.ScaleY = float32(n), float32(n)
						case "translateY":
							tr.TranslateY = style.PctLP(float32(n))
						default:
							t.Fatalf("%s: transform function %q", name, fn)
						}
					}
					f.Transform = &tr
				case "animation-timing-function":
					c, err := ParseEasing(val)
					if err != nil {
						t.Fatal(err)
					}
					f.Easing = c
				default:
					t.Fatalf("%s: property %q is not supported by the keyframes", name, prop)
				}
			}
			k.Frames = append(k.Frames, f)
		}
	}
	// Keyframes are sorted by offset (CSS applies them in that order).
	for i := 1; i < len(k.Frames); i++ {
		for j := i; j > 0 && k.Frames[j].Offset < k.Frames[j-1].Offset; j-- {
			k.Frames[j], k.Frames[j-1] = k.Frames[j-1], k.Frames[j]
		}
	}
	return k
}
