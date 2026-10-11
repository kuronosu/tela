package gfx

// outlineLoops flattens p (tolerance 1e-3) into polygons, one per subpath;
// every subpath is closed implicitly, as a fill does.
func outlineLoops(p Path) [][][2]float64 {
	var loops [][][2]float64
	p.Flatten(1e-3, func(v Verb, pt Point) {
		q := [2]float64{float64(pt.X), float64(pt.Y)}
		switch v {
		case MoveTo:
			loops = append(loops, [][2]float64{q})
		case LineTo:
			loops[len(loops)-1] = append(loops[len(loops)-1], q)
		}
	})
	return loops
}

// winding returns the winding number of the polygons around (x, y).
func winding(loops [][][2]float64, x, y float64) int {
	w := 0
	for _, l := range loops {
		n := len(l)
		for i := range n {
			a, b := l[i], l[(i+1)%n]
			if a[1] <= y {
				if b[1] > y && (b[0]-a[0])*(y-a[1])-(x-a[0])*(b[1]-a[1]) > 0 {
					w++
				}
			} else if b[1] <= y && (b[0]-a[0])*(y-a[1])-(x-a[0])*(b[1]-a[1]) < 0 {
				w--
			}
		}
	}
	return w
}

// signedArea returns the shoelace area of the polygons: the integral of the
// winding number.
func signedArea(loops [][][2]float64) float64 {
	s := 0.0
	for _, l := range loops {
		n := len(l)
		for i := range n {
			a, b := l[i], l[(i+1)%n]
			s += a[0]*b[1] - b[0]*a[1]
		}
	}
	return s / 2
}

// nonZeroArea estimates the area where the winding number is non-zero by
// sampling a grid of step h over r.
func nonZeroArea(loops [][][2]float64, r Rect, h float64) float64 {
	area := 0.0
	for y := float64(r.Y0) + h/2; y < float64(r.Y1); y += h {
		for x := float64(r.X0) + h/2; x < float64(r.X1); x += h {
			if winding(loops, x, y) != 0 {
				area += h * h
			}
		}
	}
	return area
}
