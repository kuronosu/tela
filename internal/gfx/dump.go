package gfx

import (
	"fmt"
	"io"
	"strconv"
	"strings"
)

// String returns the Dump text of the list.
func (d *DisplayList) String() string {
	var sb strings.Builder
	_ = d.Dump(&sb)
	return sb.String()
}

// Dump writes a deterministic, human-readable description of the list: one
// command per line, indented by two spaces per nesting depth, each line
// ending in a newline. Floats use the shortest representation that
// round-trips; colors are 8-bit hex. The format is meant for tests and for
// `tela inspect`, for example:
//
//	PushClipRRect rect=[0,0,100,50] r=8 bounds=[0,0,100,50]
//	  FillRect rect=[0,0,100,50] paint=#ff0000 bounds=[0,0,100,50]
//	PopClip
//
// Pop lines carry no bounds (they equal their push's). Images are shown by
// size, not by ID, so dumps do not depend on creation order.
func (d *DisplayList) Dump(w io.Writer) error {
	depth := 0
	var sb strings.Builder
	for i, c := range d.cmds {
		sb.Reset()
		if _, ok := popKind(c); ok {
			depth--
		}
		for k := 0; k < depth; k++ {
			sb.WriteString("  ")
		}
		sb.WriteString(c.Name())
		dumpArgs(&sb, c)
		if _, ok := popKind(c); !ok {
			sb.WriteString(" bounds=")
			sb.WriteString(fmtRect(d.bounds[i]))
		}
		sb.WriteByte('\n')
		if _, err := io.WriteString(w, sb.String()); err != nil {
			return err
		}
		if _, ok := pushKind(c); ok {
			depth++
		}
	}
	return nil
}

func dumpArgs(sb *strings.Builder, c Command) {
	attr := func(name, v string) {
		sb.WriteByte(' ')
		sb.WriteString(name)
		sb.WriteByte('=')
		sb.WriteString(v)
	}
	switch c := c.(type) {
	case FillRect:
		attr("rect", fmtRect(c.Rect))
		attr("paint", fmtPaint(c.Paint))
	case FillRRect:
		attr("rect", fmtRect(c.RRect.Rect))
		attr("r", fmtRadii(c.RRect))
		attr("paint", fmtPaint(c.Paint))
	case FillPath:
		attr("path", fmtPath(c.Path))
		attr("rule", c.Rule.String())
		attr("paint", fmtPaint(c.Paint))
	case StrokePath:
		attr("path", fmtPath(c.Path))
		attr("width", fmtF(c.Style.Width))
		attr("join", c.Style.Join.String())
		attr("cap", c.Style.Cap.String())
		if c.Style.MiterLimit != DefaultMiterLimit {
			attr("miter", fmtF(c.Style.MiterLimit))
		}
		if len(c.Style.Dashes) > 0 {
			attr("dashes", fmtList(c.Style.Dashes))
			attr("dashOffset", fmtF(c.Style.DashOffset))
		}
		attr("paint", fmtPaint(c.Paint))
	case DrawImage:
		attr("image", fmt.Sprintf("%dx%d", c.Image.w, c.Image.h))
		attr("src", fmtRect(c.Src))
		attr("dst", fmtRect(c.Dst))
		attr("sampling", c.Sampling.String())
	case DrawShadow:
		attr("rect", fmtRect(c.RRect.Rect))
		attr("r", fmtRadii(c.RRect))
		attr("offset", fmtPt(c.Offset))
		attr("blur", fmtF(c.Blur))
		attr("spread", fmtF(c.Spread))
		attr("color", c.Color.String())
		if c.Inset {
			sb.WriteString(" inset")
		}
	case PushClipRect:
		attr("rect", fmtRect(c.Rect))
	case PushClipRRect:
		attr("rect", fmtRect(c.RRect.Rect))
		attr("r", fmtRadii(c.RRect))
	case PushClipPath:
		attr("path", fmtPath(c.Path))
		attr("rule", c.Rule.String())
	case PushTransform:
		attr("affine", fmtList(c.Affine[:]))
	case PushLayer:
		attr("opacity", fmtF(c.Opacity))
		attr("blend", c.Blend.String())
		if len(c.Filters) > 0 {
			attr("filters", fmtFilters(c.Filters))
		}
	case BackdropFilter:
		attr("rect", fmtRect(c.Shape.Rect))
		attr("r", fmtRadii(c.Shape))
		attr("filters", fmtFilters(c.Filters))
	}
}

// fmtF formats a float32 compactly: shortest round-trip form, no exponent,
// no negative zero.
func fmtF(v float32) string {
	if v == 0 {
		return "0"
	}
	return strconv.FormatFloat(float64(v), 'f', -1, 32)
}

func fmtPt(p Point) string { return "(" + fmtF(p.X) + "," + fmtF(p.Y) + ")" }

func fmtRect(r Rect) string {
	return "[" + fmtF(r.X0) + "," + fmtF(r.Y0) + "," + fmtF(r.X1) + "," + fmtF(r.Y1) + "]"
}

func fmtList(v []float32) string {
	parts := make([]string, len(v))
	for i, x := range v {
		parts[i] = fmtF(x)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

// fmtRadii prints one value for a uniform circular RRect ("8"), "8x4" for a
// uniform elliptical one, and the four corners TL/TR/BR/BL joined by '/'
// otherwise.
func fmtRadii(rr RRect) string {
	one := func(p Point) string {
		if p.X == p.Y {
			return fmtF(p.X)
		}
		return fmtF(p.X) + "x" + fmtF(p.Y)
	}
	if rr.IsUniform() {
		return one(rr.Radii[0])
	}
	return one(rr.Radii[0]) + "/" + one(rr.Radii[1]) + "/" + one(rr.Radii[2]) + "/" + one(rr.Radii[3])
}

func fmtPath(p Path) string {
	var sb strings.Builder
	pt := func(q Point) { sb.WriteString(fmtF(q.X) + "," + fmtF(q.Y)) }
	i := 0
	for k, v := range p.verbs {
		if k > 0 {
			sb.WriteByte(' ')
		}
		switch v {
		case MoveTo:
			sb.WriteByte('M')
		case LineTo:
			sb.WriteByte('L')
		case QuadTo:
			sb.WriteByte('Q')
		case CubicTo:
			sb.WriteByte('C')
		case Close:
			sb.WriteByte('Z')
		}
		for j := 0; j < v.NumPoints(); j++ {
			if j > 0 {
				sb.WriteByte(' ')
			}
			pt(p.pts[i])
			i++
		}
	}
	return "{" + sb.String() + "}"
}

func fmtStops(stops []GradientStop) string {
	parts := make([]string, len(stops))
	for i, s := range stops {
		parts[i] = fmtF(s.Offset) + ":" + s.Color.String()
	}
	return "[" + strings.Join(parts, " ") + "]"
}

func fmtPaint(p Paint) string {
	switch p := p.(type) {
	case Color:
		return p.String()
	case LinearGradient:
		return "linear(" + fmtPt(p.Start) + "->" + fmtPt(p.End) + " " + p.Interp.String() + " " + fmtStops(p.Stops) + ")"
	case RadialGradient:
		return "radial(c=" + fmtPt(p.Center) + " r=" + fmtPt(p.Radius) + " " + p.Interp.String() + " " + fmtStops(p.Stops) + ")"
	case ConicGradient:
		return "conic(c=" + fmtPt(p.Center) + " from=" + fmtF(p.StartAngle) + " " + p.Interp.String() + " " + fmtStops(p.Stops) + ")"
	}
	return "paint(?)"
}

func fmtFilters(fs []Filter) string {
	parts := make([]string, len(fs))
	for i, f := range fs {
		switch f := f.(type) {
		case Blur:
			parts[i] = "blur(" + fmtF(f.StdDev) + ")"
		case DropShadow:
			parts[i] = "drop-shadow(" + fmtPt(f.Offset) + " " + fmtF(f.StdDev) + " " + f.Color.String() + ")"
		case ColorMatrix:
			parts[i] = "matrix" + fmtList(f.M[:])
		}
	}
	return "[" + strings.Join(parts, " ") + "]"
}
