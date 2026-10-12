package style

import (
	"fmt"
	"strconv"
	"strings"
)

func registerFlexGrid(r *registry) {
	// flex-* : direction, wrap and the flex shorthand.
	r.add("flex", false, func(c *pctx, a *arg) (Op, error) {
		if a.hasMod {
			return Op{}, errNoMatch
		}
		v := a.value
		if a.arb {
			return flexShorthand(c, a.inner)
		}
		switch v {
		case "row":
			return op(func(e *env) { e.s.FlexDirection = FlexRow }), nil
		case "row-reverse":
			return op(func(e *env) { e.s.FlexDirection = FlexRowReverse }), nil
		case "col":
			return op(func(e *env) { e.s.FlexDirection = FlexColumn }), nil
		case "col-reverse":
			return op(func(e *env) { e.s.FlexDirection = FlexColumnReverse }), nil
		case "wrap":
			return op(func(e *env) { e.s.FlexWrap = FlexWrapOn }), nil
		case "wrap-reverse":
			return op(func(e *env) { e.s.FlexWrap = FlexWrapReverse }), nil
		case "nowrap":
			return op(func(e *env) { e.s.FlexWrap = FlexNoWrap }), nil
		case "auto":
			return flexOp(1, 1, Auto), nil
		case "initial":
			return flexOp(0, 1, Auto), nil
		case "none":
			return flexOp(0, 0, Auto), nil
		}
		if n, ok := parseFloat(v); ok && n >= 0 && v[0] != '+' {
			return flexOp(n, 1, DimPct(0)), nil
		}
		return Op{}, errNoMatch
	}, func(*Theme) []string {
		return []string{"row", "row-reverse", "col", "col-reverse", "wrap", "wrap-reverse", "nowrap",
			"1", "2", "3", "auto", "initial", "none"}
	})

	flexNum := func(root string, set func(*env, float32)) {
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			if a.raw == "" {
				return op(func(e *env) { set(e, 1) }), nil
			}
			n, ok := numberArg(a)
			if !ok || n < 0 {
				return Op{}, errNoMatch
			}
			return op(func(e *env) { set(e, n) }), nil
		}, func(*Theme) []string { return []string{"", "0", "1"} })
	}
	flexNum("grow", func(e *env, n float32) { e.s.FlexGrow = n })
	flexNum("shrink", func(e *env, n float32) { e.s.FlexShrink = n })
	r.alias("flex-grow", "grow")
	r.alias("flex-grow-0", "grow-0")
	r.alias("flex-shrink", "shrink")
	r.alias("flex-shrink-0", "shrink-0")

	r.dimUtil("basis", dimAuto|dimFull|dimFrac|dimSpacing|dimIntrinsic|dimContainer, axisX,
		func(e *env, d Dim) { e.s.FlexBasis = d })

	r.add("order", true, func(c *pctx, a *arg) (Op, error) {
		var n int32
		switch a.raw {
		case "first", "last", "none":
			if a.neg {
				return Op{}, errNoMatch
			}
		}
		switch a.raw {
		case "first":
			n = -9999
		case "last":
			n = 9999
		case "none":
			n = 0
		default:
			f, ok := numberArg(a)
			if !ok || f != float32(int32(f)) {
				return Op{}, errNoMatch
			}
			n = int32(f)
			if a.neg {
				n = -n
			}
		}
		return op(func(e *env) { e.s.Order = n }), nil
	}, func(*Theme) []string { return []string{"first", "last", "none", "1", "2", "3"} })

	// Alignment.
	type al = Align
	content := map[string]al{"normal": AlignAuto, "center": AlignCenter, "start": AlignFlexStart,
		"end": AlignFlexEnd, "between": AlignSpaceBetween, "around": AlignSpaceAround,
		"evenly": AlignSpaceEvenly, "baseline": AlignBaseline, "stretch": AlignStretch}
	items := map[string]al{"start": AlignFlexStart, "end": AlignFlexEnd, "center": AlignCenter,
		"baseline": AlignBaseline, "stretch": AlignStretch}
	self := map[string]al{"auto": AlignAuto, "start": AlignFlexStart, "end": AlignFlexEnd,
		"center": AlignCenter, "stretch": AlignStretch, "baseline": AlignBaseline}
	jitems := map[string]al{"normal": AlignAuto, "start": AlignStart, "end": AlignEnd,
		"center": AlignCenter, "stretch": AlignStretch}
	jself := map[string]al{"auto": AlignAuto, "start": AlignStart, "end": AlignEnd,
		"center": AlignCenter, "stretch": AlignStretch}
	justify := map[string]al{"normal": AlignAuto, "start": AlignFlexStart, "end": AlignFlexEnd,
		"center": AlignCenter, "between": AlignSpaceBetween, "around": AlignSpaceAround,
		"evenly": AlignSpaceEvenly, "stretch": AlignStretch, "baseline": AlignBaseline}

	alignEnum := func(root string, vals map[string]al, set func(*env, Align)) {
		keys := map[string]Op{}
		for _, k := range sortedMapKeys(vals) {
			v := vals[k]
			keys[k] = op(func(e *env) { set(e, v) })
		}
		r.enum(root, false, keys)
	}
	alignEnum("justify", justify, func(e *env, a Align) { e.s.JustifyContent = a })
	alignEnum("items", mergeAlign(items, map[string]al{"normal": AlignAuto}), func(e *env, a Align) { e.s.AlignItems = a })
	alignEnum("content", content, func(e *env, a Align) { e.s.AlignContent = a })
	alignEnum("self", self, func(e *env, a Align) { e.s.AlignSelf = a })
	alignEnum("justify-items", jitems, func(e *env, a Align) { e.s.JustifyItems = a })
	alignEnum("justify-self", jself, func(e *env, a Align) { e.s.JustifySelf = a })
	alignEnum("place-content", content, func(e *env, a Align) { e.s.AlignContent, e.s.JustifyContent = a, a })
	alignEnum("place-items", mergeAlign(map[string]al{"start": AlignStart, "end": AlignEnd, "center": AlignCenter,
		"baseline": AlignBaseline, "stretch": AlignStretch}, nil), func(e *env, a Align) { e.s.AlignItems, e.s.JustifyItems = a, a })
	alignEnum("place-self", jself, func(e *env, a Align) { e.s.AlignSelf, e.s.JustifySelf = a, a })

	registerGrid(r)
}

func mergeAlign(a, b map[string]Align) map[string]Align {
	out := map[string]Align{}
	for k, v := range a {
		out[k] = v
	}
	for k, v := range b {
		out[k] = v
	}
	return out
}

func flexOp(grow, shrink float32, basis Dim) Op {
	return op(func(e *env) {
		e.s.FlexGrow, e.s.FlexShrink, e.s.FlexBasis = grow, shrink, basis
	})
}

// flexShorthand parses an arbitrary flex value: "2", "2 2 0%", "1 1 auto",
// "30%".
func flexShorthand(c *pctx, inner string) (Op, error) {
	f := strings.Fields(inner)
	var nums []float32
	basis := DimPct(0)
	hasBasis := false
	for _, w := range f {
		if n, ok := parseFloat(w); ok && !hasBasis {
			nums = append(nums, n)
			continue
		}
		if w == "auto" {
			basis, hasBasis = Auto, true
			continue
		}
		if e, err := parseLengthExpr(w, true); err == nil {
			lp, err := trackLP(e, c.th.RootFontSize)
			if err != nil {
				return Op{}, err
			}
			basis, hasBasis = DimOf(lp), true
			continue
		}
		return Op{}, fmt.Errorf("invalid flex value %q; use e.g. [2_2_0%%], [1_1_auto] or [30%%]", inner)
	}
	if len(nums) == 0 || len(nums) > 2 {
		if len(nums) == 0 && hasBasis {
			nums = []float32{1, 1}
		} else {
			return Op{}, fmt.Errorf("invalid flex value %q; use e.g. [2_2_0%%], [1_1_auto] or [30%%]", inner)
		}
	}
	grow, shrink := nums[0], float32(1)
	if len(nums) == 2 {
		shrink = nums[1]
	}
	return flexOp(grow, shrink, basis), nil
}

func registerGrid(r *registry) {
	cols := func(root string, set func(*env, TrackList)) {
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			var list TrackList
			switch {
			case a.arb:
				if a.hint != "" {
					return Op{}, errNoMatch
				}
				l, err := parseTrackList(a.inner, c.th.RootFontSize)
				if err != nil {
					return Op{}, err
				}
				list = l
			case a.value == "none":
			case a.value == "subgrid":
				return Op{}, unsupportedErr("subgrid is not supported; declare the tracks again on the nested grid")
			default:
				n, ok := parseUint(a.value)
				if !ok {
					return Op{}, errNoMatch
				}
				if n < 1 || n > 1000 {
					return Op{}, fmt.Errorf("the track count must be from 1 to 1000, got %d", n)
				}
				list = TrackList{{Repeat: true, Kind: RepeatCount, Count: n, Tracks: []TrackSize{{Max: TrackBreadth{Kind: BreadthFr, Fr: 1}, Min: TrackBreadth{Kind: BreadthLength}}}}}
			}
			return op(func(e *env) { set(e, list) }), nil
		}, func(*Theme) []string {
			out := []string{"none"}
			for i := 1; i <= 12; i++ {
				out = append(out, strconv.Itoa(i))
			}
			return out
		})
	}
	cols("grid-cols", func(e *env, l TrackList) { e.s.GridTemplateColumns = l })
	cols("grid-rows", func(e *env, l TrackList) { e.s.GridTemplateRows = l })

	flow := map[string]GridAutoFlow{"row": GridFlowRow, "col": GridFlowColumn, "dense": GridFlowRowDense,
		"row-dense": GridFlowRowDense, "col-dense": GridFlowColumnDense}
	flowOps := map[string]Op{}
	for _, k := range sortedMapKeys(flow) {
		v := flow[k]
		flowOps[k] = op(func(e *env) { e.s.GridAutoFlow = v })
	}
	r.enum("grid-flow", false, flowOps)

	autoTracks := func(root string, set func(*env, []TrackSize)) {
		r.add(root, false, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			var ts []TrackSize
			switch {
			case a.arb:
				l, err := parseTrackList(a.inner, c.th.RootFontSize)
				if err != nil {
					return Op{}, err
				}
				ts = l.Expand()
			case a.value == "auto":
				ts = []TrackSize{{}}
			case a.value == "min":
				ts = []TrackSize{{Min: TrackBreadth{Kind: BreadthMinContent}, Max: TrackBreadth{Kind: BreadthMinContent}}}
			case a.value == "max":
				ts = []TrackSize{{Min: TrackBreadth{Kind: BreadthMaxContent}, Max: TrackBreadth{Kind: BreadthMaxContent}}}
			case a.value == "fr":
				ts = []TrackSize{{Max: TrackBreadth{Kind: BreadthFr, Fr: 1}}}
			default:
				return Op{}, errNoMatch
			}
			return op(func(e *env) { set(e, ts) }), nil
		}, func(*Theme) []string { return []string{"auto", "min", "max", "fr"} })
	}
	autoTracks("auto-cols", func(e *env, t []TrackSize) { e.s.GridAutoColumns = t })
	autoTracks("auto-rows", func(e *env, t []TrackSize) { e.s.GridAutoRows = t })

	placement := func(prefix string, get func(*ComputedStyle) *GridPlacement) {
		line := func(name string, set func(*GridPlacement, GridLine)) {
			r.add(prefix+"-"+name, false, func(c *pctx, a *arg) (Op, error) {
				if a.hasMod {
					return Op{}, errNoMatch
				}
				var ln GridLine
				switch {
				case a.arb:
					l, err := parseGridLine(a.inner)
					if err != nil {
						return Op{}, err
					}
					ln = l
				case a.value == "auto":
				default:
					n, ok := parseUint(a.value)
					if !ok || n == 0 {
						return Op{}, errNoMatch
					}
					ln = GridLine{GridLineIndex, n}
				}
				return op(func(e *env) { set(get(e.s), ln) }), nil
			}, func(*Theme) []string {
				return []string{"auto", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12", "13"}
			})
		}
		line("start", func(p *GridPlacement, l GridLine) { p.Start = l })
		line("end", func(p *GridPlacement, l GridLine) { p.End = l })

		r.add(prefix+"-span", false, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			var pl GridPlacement
			switch {
			case a.value == "full":
				pl = GridPlacement{GridLine{GridLineIndex, 1}, GridLine{GridLineIndex, -1}}
			default:
				var n int
				var ok bool
				if a.arb {
					n, ok = parseUint(a.inner)
				} else {
					n, ok = parseUint(a.value)
				}
				if !ok || n == 0 {
					return Op{}, errNoMatch
				}
				pl = GridPlacement{GridLine{GridLineSpan, n}, GridLine{GridLineSpan, n}}
			}
			return op(func(e *env) { *get(e.s) = pl }), nil
		}, func(*Theme) []string {
			return []string{"full", "1", "2", "3", "4", "5", "6", "7", "8", "9", "10", "11", "12"}
		})

		r.add(prefix, false, func(c *pctx, a *arg) (Op, error) {
			if a.hasMod {
				return Op{}, errNoMatch
			}
			var pl GridPlacement
			switch {
			case a.value == "auto":
			case a.arb:
				p, err := parseGridPlacement(a.inner)
				if err != nil {
					return Op{}, err
				}
				pl = p
			default:
				return Op{}, errNoMatch
			}
			return op(func(e *env) { *get(e.s) = pl }), nil
		}, func(*Theme) []string { return []string{"auto"} })
	}
	placement("col", func(s *ComputedStyle) *GridPlacement { return &s.GridColumn })
	placement("row", func(s *ComputedStyle) *GridPlacement { return &s.GridRow })
}
