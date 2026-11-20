package core

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/kuronosu/tela/internal/gfx"
)

func TestInspectNodeJSONRoundTrip(t *testing.T) {
	b := gfx.RectXYWH(5, 6, 70, 80)
	in := InspectNode{
		Kind: "div", Key: "k", Component: "main.Row", Element: 7, Src: "settings.go:42",
		Rect: gfx.RectXYWH(1, 2, 30, 40), Bounds: &b,
		Margin: Edges{Top: 1, Right: 2, Bottom: 3, Left: 4}, Padding: Edges{Top: 8, Right: 8, Bottom: 8, Left: 8},
		Text: "hi", Lines: 1, Inline: []string{"span"}, InlineBox: true,
		Fragments: []InspectFragment{{Inline: 0, Line: 1, Rect: gfx.RectXYWH(9, 9, 10, 10)}},
		Style:     StyleSummary{Display: "flex", Color: "#000", Font: "400 14px/20 Inter", Opacity: 1},
		Children:  []InspectNode{{Kind: "span", Rect: gfx.RectXYWH(3, 3, 4, 4), Style: StyleSummary{Display: "inline", Opacity: 1}}},
	}
	data, err := json.Marshal(in)
	if err != nil {
		t.Fatal(err)
	}
	var out InspectNode
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Errorf("round trip:\n in  %+v\n out %+v", in, out)
	}
}
