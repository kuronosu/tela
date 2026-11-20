package core

import (
	"encoding/json"

	"github.com/kuronosu/tela/internal/gfx"
)

// UnmarshalJSON reads what [InspectNode.MarshalJSON] writes: rect and bounds
// as {x, y, width, height}. The tools socket of tela dev (ADR-299) sends a
// node this way and tela devtools reads it back.
func (n *InspectNode) UnmarshalJSON(b []byte) error {
	var a struct {
		Kind      string        `json:"kind"`
		Key       string        `json:"key"`
		Component string        `json:"component"`
		Element   ElementID     `json:"element"`
		Src       string        `json:"src"`
		Rect      jsonRect      `json:"rect"`
		Bounds    *jsonRect     `json:"bounds"`
		Margin    Edges         `json:"margin"`
		Border    Edges         `json:"border"`
		Padding   Edges         `json:"padding"`
		Text      string        `json:"text"`
		Lines     int           `json:"lines"`
		Inline    []string      `json:"inline"`
		Fragments []jsonFrag    `json:"fragments"`
		InlineBox bool          `json:"inlineBox"`
		Style     StyleSummary  `json:"style"`
		Children  []InspectNode `json:"children"`
	}
	if err := json.Unmarshal(b, &a); err != nil {
		return err
	}
	rect := func(r jsonRect) gfx.Rect { return gfx.RectXYWH(r.X, r.Y, r.Width, r.Height) }
	*n = InspectNode{
		Kind: a.Kind, Key: a.Key, Component: a.Component, Element: a.Element, Src: a.Src,
		Rect: rect(a.Rect), Margin: a.Margin, Border: a.Border, Padding: a.Padding,
		Text: a.Text, Lines: a.Lines, Inline: a.Inline, InlineBox: a.InlineBox,
		Style: a.Style, Children: a.Children,
	}
	if a.Bounds != nil {
		r := rect(*a.Bounds)
		n.Bounds = &r
	}
	for _, f := range a.Fragments {
		n.Fragments = append(n.Fragments, InspectFragment{Inline: f.Inline, Line: f.Line, Rect: rect(f.Rect)})
	}
	return nil
}
