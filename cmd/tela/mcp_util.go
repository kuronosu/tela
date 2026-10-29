package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"slices"
)

// layoutBox is what layoutSubtree reads of a node of the layout JSON.
type layoutBox struct {
	Kind     string            `json:"kind"`
	Rect     layoutRect        `json:"rect"`
	Bounds   *layoutRect       `json:"bounds"`
	Children []json.RawMessage `json:"children"`
}

type layoutRect struct{ X, Y, Width, Height float64 }

// box is where the node is seen: its bounds when it is painted elsewhere,
// else its rect.
func (b *layoutBox) box() layoutRect {
	if b.Bounds != nil {
		return *b.Bounds
	}
	return b.Rect
}

// roleKinds are the primitives that have a role implicitly, to choose
// among boxes with the same rectangle.
var roleKinds = map[string][]string{
	"button":    {"button"},
	"link":      {"a"},
	"textbox":   {"input", "textarea"},
	"searchbox": {"input"},
	"heading":   {"h1", "h2", "h3", "h4", "h5", "h6"},
	"img":       {"img"},
	"list":      {"ul", "ol"},
	"listitem":  {"li"},
	"paragraph": {"p"},
	"form":      {"form"},
	"separator": {"hr"},
}

// layoutSubtree returns, indented, the node of the layout JSON js whose
// box is the element's bounds (x, y, width, height): the outermost such
// node, or the one whose primitive has the element's role. Without an
// exact match it returns the innermost node that contains the bounds.
func layoutSubtree(js []byte, bounds [4]float64, role string) ([]byte, error) {
	want := layoutRect{bounds[0], bounds[1], bounds[2], bounds[3]}
	same := func(a, b layoutRect) bool {
		const eps = 0.01
		return math.Abs(a.X-b.X) < eps && math.Abs(a.Y-b.Y) < eps && math.Abs(a.Width-b.Width) < eps && math.Abs(a.Height-b.Height) < eps
	}
	contains := func(a, b layoutRect) bool {
		const eps = 0.5
		return a.X-eps <= b.X && a.Y-eps <= b.Y && b.X+b.Width <= a.X+a.Width+eps && b.Y+b.Height <= a.Y+a.Height+eps
	}
	var exact, typed, inner json.RawMessage
	var walk func(raw json.RawMessage) error
	walk = func(raw json.RawMessage) error {
		var b layoutBox
		if err := json.Unmarshal(raw, &b); err != nil {
			return err
		}
		box := b.box()
		if same(box, want) {
			if exact == nil {
				exact = raw
			}
			if typed == nil && slices.Contains(roleKinds[role], b.Kind) {
				typed = raw
			}
		}
		if b.Kind != "root" && contains(box, want) {
			inner = raw
		}
		for _, c := range b.Children {
			if err := walk(c); err != nil {
				return err
			}
		}
		return nil
	}
	if err := walk(js); err != nil {
		return nil, err
	}
	pick := typed
	if pick == nil {
		pick = exact
	}
	if pick == nil {
		pick = inner
	}
	if pick == nil {
		return nil, errors.New("no box has the element's bounds")
	}
	var compact, out bytes.Buffer
	if err := json.Compact(&compact, pick); err != nil {
		return nil, err
	}
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, err
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}
