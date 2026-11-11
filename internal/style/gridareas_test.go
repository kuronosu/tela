package style

import (
	"reflect"
	"strings"
	"testing"
)

// grid-template-areas and grid-area as arbitrary properties: Tailwind v4
// has no utility for them (ADR-268).

func TestGridTemplateAreasClass(t *testing.T) {
	for _, c := range []struct {
		class string
		want  GridAreas
	}{
		{"[grid-template-areas:'head_head'_'side_main']", GridAreas{
			Template: []string{"head head", "side main"},
			Areas: []GridArea{
				{Name: "head", RowStart: 1, RowEnd: 2, ColumnStart: 1, ColumnEnd: 3},
				{Name: "side", RowStart: 2, RowEnd: 3, ColumnStart: 1, ColumnEnd: 2},
				{Name: "main", RowStart: 2, RowEnd: 3, ColumnStart: 2, ColumnEnd: 3},
			},
			Rows: 2, Columns: 2,
		}},
		// Double quotes, unnamed cells (one or more dots), an area that
		// spans rows, several spaces, and an escaped underscore in a name.
		{`[grid-template-areas:"a_._side\_bar"_"b_..._side\_bar"]`, GridAreas{
			Template: []string{"a . side_bar", "b . side_bar"},
			Areas: []GridArea{
				{Name: "a", RowStart: 1, RowEnd: 2, ColumnStart: 1, ColumnEnd: 2},
				{Name: "side_bar", RowStart: 1, RowEnd: 3, ColumnStart: 3, ColumnEnd: 4},
				{Name: "b", RowStart: 2, RowEnd: 3, ColumnStart: 1, ColumnEnd: 2},
			},
			Rows: 2, Columns: 3,
		}},
		{"[grid-template-areas:'x'] [grid-template-areas:none]", GridAreas{}},
		{"max-sm:[grid-template-areas:'a_b']", GridAreas{}}, // the 1280 px window is not below sm
	} {
		got := style(t, c.class).GridTemplateAreas
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: %+v, want %+v", c.class, got, c.want)
		}
	}
	if s := style(t, "[grid-template-areas:'head_head'_'side_main']").GridTemplateAreas.String(); s != "'head head' 'side main'" {
		t.Errorf("String = %q", s)
	}
	if s := (GridAreas{}).String(); s != "none" {
		t.Errorf("none String = %q", s)
	}
}

func TestGridTemplateAreasErrors(t *testing.T) {
	for _, c := range []struct{ class, msg string }{
		{"[grid-template-areas:'a_b'_'c']", "same number of cells"},
		{"[grid-template-areas:'a_b'_'b_a']", "not a rectangle"},
		{"[grid-template-areas:'a_b'_'c_a']", "not a rectangle"},
		{"[grid-template-areas:a_b]", "one quoted string per row"},
		{"[grid-template-areas:'a_b]", "unterminated"},
		{"[grid-template-areas:'']", "empty row"},
		{"[grid-template-areas:'1a']", "invalid area name"},
	} {
		cl := Compile(c.class)
		if cl.Err == nil {
			t.Errorf("%s is accepted", c.class)
			continue
		}
		if !strings.Contains(cl.Err.Error(), c.msg) {
			t.Errorf("%s: %v, want an error about %q", c.class, cl.Err, c.msg)
		}
	}
}

func TestGridAreaClass(t *testing.T) {
	line := func(k GridLineKind, n int) GridLine { return GridLine{Kind: k, N: n} }
	main := GridLine{Kind: GridLineNamed, Name: "main"}
	side := GridLine{Kind: GridLineNamed, Name: "side"}
	for _, c := range []struct {
		class    string
		row, col GridPlacement
	}{
		// A lone name fills every omitted line (CSS Grid 1 section 8.4).
		{"[grid-area:main]", GridPlacement{main, main}, GridPlacement{main, main}},
		{"[grid-area:main/side]", GridPlacement{main, main}, GridPlacement{side, side}},
		{"[grid-area:main/side/2]", GridPlacement{main, line(GridLineIndex, 2)}, GridPlacement{side, side}},
		{"[grid-area:1/2/3/4]", GridPlacement{line(GridLineIndex, 1), line(GridLineIndex, 3)}, GridPlacement{line(GridLineIndex, 2), line(GridLineIndex, 4)}},
		// A number leaves the omitted lines auto.
		{"[grid-area:2]", GridPlacement{Start: line(GridLineIndex, 2)}, GridPlacement{}},
		{"[grid-area:span_2_/_3]", GridPlacement{Start: line(GridLineSpan, 2)}, GridPlacement{Start: line(GridLineIndex, 3)}},
		// Later classes win per property, as in CSS.
		{"[grid-area:main] col-span-2", GridPlacement{main, main}, GridPlacement{line(GridLineSpan, 2), line(GridLineSpan, 2)}},
	} {
		s := style(t, c.class)
		if s.GridRow != c.row || s.GridColumn != c.col {
			t.Errorf("%s: row %v, col %v; want row %v, col %v", c.class, s.GridRow, s.GridColumn, c.row, c.col)
		}
	}
	for _, bad := range []string{"[grid-area:1/2/3/4/5]", "[grid-area:span]", "[grid-area:0]"} {
		if c := Compile(bad); c.Err == nil {
			t.Errorf("%s is accepted", bad)
		}
	}
}
