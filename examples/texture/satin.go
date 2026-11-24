package main

// satinShader is shaders/satin.glsl: folds of satin under a warm light and a
// cool one.
type satinShader struct {
	ax, ay, bx, by     float32 // where the noises are
	phase, phase2      float32
	warm, cool         vec3
	warmHalf, coolHalf vec3 // between the light and the viewer

	a, b [3]noise2
	grid grid
}

func newSatin(u *uniforms) *satinShader {
	s := &satinShader{
		ax: u.V[0][0], ay: u.V[0][1], bx: u.V[0][2], by: u.V[0][3],
		phase: u.V[1][0], phase2: u.V[1][1], warm: xyz(u.V[2]), cool: xyz(u.V[3]),
	}
	s.warmHalf = vec3{s.warm.x, s.warm.y, s.warm.z + 1}.unit()
	s.coolHalf = vec3{s.cool.x, s.cool.y, s.cool.z + 1}.unit()
	s.grid = newGrid(u, s.folds)
	return s
}

// folds returns the height of the cloth, and minus its slopes: the noise a
// bends the noise b, and both bend the folds.
func (s *satinShader) folds(x, y float32) vec3 {
	a, ax, ay := slopes(&s.a, float32(x*0.95)+s.ax, float32(y*1.05)+s.ay)
	ax, ay = float32(ax*0.95), float32(ay*1.05)
	b, bu, bv := slopes(&s.b, float32(x*0.9)+float32(2.2*a)+4+s.bx, y+float32(1.8*a)+1+s.by)
	bx := float32(bu*(0.9+float32(2.2*ax))) + float32(bv*float32(1.8*ax))
	by := float32(bu*float32(2.2*ay)) + float32(bv*(1+float32(1.8*ay)))
	f := float32(1.1*x) - float32(1.5*y) + float32(2.6*b) + float32(1.2*a)
	fx := 1.1 + float32(2.6*bx) + float32(1.2*ax)
	fy := float32(2.6*by) + float32(1.2*ay) - 1.5
	p := float32(0.4934*f) + s.phase
	q := float32(0.2069*f) - float32(0.1273*x) + float32(0.3183*a) - s.phase2
	h := float32(0.275*sine(p)) + float32(0.30*sine(q)) + float32(0.18*b)
	k, l := float32(0.8525*cosine(p)), float32(0.30*cosine(q))
	hx := float32(k*fx) + float32(l*(float32(1.3*fx)-0.8+float32(2*ax))) + float32(0.18*bx)
	hy := float32(k*fy) + float32(l*(float32(1.3*fy)+float32(2*ay))) + float32(0.18*by)
	return vec3{h, float32(hx * -0.30), float32(hy * -0.30)}
}

func (s *satinShader) pixel(col, row int, x, y float32) vec3 {
	g := s.grid.seek(col, row)
	h, n := g.x, vec3{g.y, g.z, 1}.unit()

	c := mix(vec3{0.007, 0.0075, 0.0088}, vec3{0.054, 0.053, 0.052}, smooth(-0.55, 0.60, h))
	c = c.mul(0.13 + float32(1.35*pos(n.dot(s.warm))) + float32(0.60*pos(n.dot(s.cool))))
	g2, g8, g32 := powers(n.dot(s.warmHalf))
	c = c.plus(vec3{1, 0.82, 0.62}, float32(0.46*float32(g32*float32(g8*g8)))+float32(0.080*float32(g8*g2)))
	_, g8, g32 = powers(n.dot(s.coolHalf))
	c = c.plus(vec3{0.50, 0.66, 0.85}, float32(0.22*float32(g32*g32))+float32(0.040*g8))
	rim := 1 - n.z
	c = c.plus(vec3{0.8, 0.72, 0.62}, float32(0.09*float32(rim*rim)))
	return c.mul(1 - float32(0.19*float32(x*x)) - float32(0.46*float32(y*y)))
}

func pos(x float32) float32 {
	if x < 0 {
		return 0
	}
	return x
}

// powers returns x to the power of 2, of 8 and of 32.
func powers(x float32) (p2, p8, p32 float32) {
	x = pos(x)
	p2 = float32(x * x)
	p4 := float32(p2 * p2)
	p8 = float32(p4 * p4)
	p16 := float32(p8 * p8)
	return p2, p8, float32(p16 * p16)
}

// slopes is fbm2 with its derivatives.
func slopes(n *[3]noise2, x, y float32) (v, dx, dy float32) {
	x, y = float32(x), float32(y)
	var a float32 = 0.5
	// How x and y change with the first ones.
	var xx, xy, yx, yy float32 = 1, 0, 0, 1
	for i := range n {
		o, ox, oy := n[i].slope(x, y)
		v += float32(a * o)
		dx += float32(a * (float32(ox*xx) + float32(oy*yx)))
		dy += float32(a * (float32(ox*xy) + float32(oy*yy)))
		x, y = float32(1.6*x)+float32(1.2*y)+3.1, float32(1.6*y)-float32(1.2*x)+7.7
		xx, yx = float32(1.6*xx)+float32(1.2*yx), float32(1.6*yx)-float32(1.2*xx)
		xy, yy = float32(1.6*xy)+float32(1.2*yy), float32(1.6*yy)-float32(1.2*xy)
		a *= 0.5
	}
	return
}
