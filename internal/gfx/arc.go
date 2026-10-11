// Copyright 2019 the Kurbo Authors
// SPDX-License-Identifier: Apache-2.0 OR MIT
//
// Ported to Go from kurbo's kurbo/src/arc.rs (Arc::append_iter and
// sample_ellipse). See THIRD_PARTY.md.

package gfx

import (
	"math"

	"github.com/kuronosu/tela/internal/gfx/internal/fmath"
)

// defaultArcTolerance is the maximum distance, in logical pixels, between a
// built-in circle, ellipse or rounded-corner path and the true curve.
const defaultArcTolerance = 0.01

// maxArcSegments bounds the number of cubics generated for one arc so that
// absurd tolerances cannot exhaust memory.
const maxArcSegments = 4096

// arcResolution is the relative float32 resolution below which an arc
// tolerance is meaningless: about two ulps of the largest output coordinate.
const arcResolution = 0x1p-22

// Arc is an elliptical arc segment in the style of kurbo. Angles are in
// radians; in the y-down coordinate system a positive sweep runs clockwise on
// screen, and angle 0 points along +X (right).
type Arc struct {
	Center Point
	// Radii holds the ellipse radii: X along the ellipse's x axis, Y along
	// its y axis, before XRotation is applied.
	Radii      Point
	StartAngle float32
	// SweepAngle is the angle between the start and the end of the arc. It
	// may be negative.
	SweepAngle float32
	// XRotation rotates the ellipse about its center.
	XRotation float32
}

// eval returns the point of the arc at parameter t in [0, 1].
func (a Arc) eval(t float64) Point {
	ang := f64(a.StartAngle) + float64(f64(a.SweepAngle)*t)
	v := sampleEllipse(f64(a.Radii.X), f64(a.Radii.Y), f64(a.XRotation), ang)
	return Point{float32(f64(a.Center.X) + v[0]), float32(f64(a.Center.Y) + v[1])}
}

// addArcCubics appends the cubic Beziers approximating a to p. The current
// point of p must already be the start of the arc; no line or move is added.
// This is kurbo's Arc::append_iter.
//
// The tolerance (defaultArcTolerance if not positive) is raised to the
// float32 resolution of the arc's coordinates (arcResolution times the
// largest of them), below which more segments cannot add accuracy. This
// bounds the segment count for huge radii to about 13 per full turn instead
// of thousands. All arithmetic is deterministic (fmath.Sincos, explicit
// rounding of every product), so the points are the same on every
// architecture.
func (p *Path) addArcCubics(a Arc, tolerance float32) {
	tol := f64(tolerance)
	if !(tol > 0) {
		tol = defaultArcTolerance
	}
	sweep := f64(a.SweepAngle)
	rx, ry := f64(a.Radii.X), f64(a.Radii.Y)
	if !(math.Abs(sweep) > 0) || math.IsInf(sweep, 0) {
		return
	}
	cx, cy, xr := f64(a.Center.X), f64(a.Center.Y), f64(a.XRotation)
	rmax := math.Max(math.Abs(rx), math.Abs(ry))
	tol = math.Max(tol, (math.Max(math.Abs(cx), math.Abs(cy))+rmax)*arcResolution)
	sign := 1.0
	if sweep < 0 {
		sign = -1
	}
	scaledErr := rmax / tol
	// Number of subdivisions per ellipse based on error tolerance. This may
	// slightly underestimate the error for quadrants (as in kurbo). The sixth
	// root is a cube root of a square root, both deterministic.
	nErr := math.Max(fmath.Cbrt(math.Sqrt(1.1163*scaledErr)), 3.999999)
	nf := math.Ceil(nErr * math.Abs(sweep) * (1 / (2 * math.Pi)))
	if math.IsNaN(nf) || nf < 1 {
		nf = 1
	}
	if nf > maxArcSegments {
		nf = maxArcSegments
	}
	n := int(nf)
	step := sweep / nf
	ts, tc := fmath.Sincos(0.25 * step)
	arm := (4.0 / 3.0) * math.Abs(ts/tc) * sign
	angle0 := f64(a.StartAngle)
	p0 := sampleEllipse(rx, ry, xr, angle0)
	for i := 0; i < n; i++ {
		angle1 := angle0 + step
		d0 := sampleEllipse(rx, ry, xr, angle0+math.Pi/2)
		p3 := sampleEllipse(rx, ry, xr, angle1)
		d1 := sampleEllipse(rx, ry, xr, angle1+math.Pi/2)
		p.CubicTo(
			Point{float32(cx + p0[0] + float64(arm*d0[0])), float32(cy + p0[1] + float64(arm*d0[1]))},
			Point{float32(cx + p3[0] - float64(arm*d1[0])), float32(cy + p3[1] - float64(arm*d1[1]))},
			Point{float32(cx + p3[0]), float32(cy + p3[1])},
		)
		angle0 = angle1
		p0 = p3
	}
}

// sampleEllipse returns the point of an ellipse with radii (rx, ry) rotated
// by xRotation, at the given angle, relative to the center.
func sampleEllipse(rx, ry, xRotation, angle float64) [2]float64 {
	s, c := fmath.Sincos(angle)
	u, v := rx*c, ry*s
	rs, rc := fmath.Sincos(xRotation)
	return [2]float64{float64(u*rc) - float64(v*rs), float64(u*rs) + float64(v*rc)}
}
