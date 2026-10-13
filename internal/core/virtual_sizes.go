package core

// sizeIndex holds the sizes of the items of a virtual list (virtual.go) and
// their prefix sums in a Fenwick (binary indexed) tree: the offset of an
// item, the total and the item at an offset take O(log n), a size change
// O(log n), and setting every size O(n).
//
// Sizes are float32 logical pixels, as layout produces them; the sums are
// float64. Sums of float32 values in float64 are exact for any realistic
// list (each float32 has a 24-bit mantissa, so a sum stays exact while it
// is below 2^29 times the smallest fractional step of its terms), which
// makes every prefix sum independent of the order of its additions, and
// there are no products, so nothing here can be fused into a multiply-add
// (ADR-041): offsets are the same on every architecture.
type sizeIndex struct {
	size []float32
	// tree is 1-based: tree[k] is the sum of size[k-lowbit(k) : k].
	tree []float64
	// step is the largest power of two not above len(size), where
	// searches start.
	step int
}

// maxItemSize bounds item sizes (logical px), so that a wrong estimate
// cannot make the content infinitely tall.
const maxItemSize = 1 << 20

// clampSize returns v as an item size: NaN and values below zero are 0,
// values above maxItemSize are maxItemSize.
func clampSize(v float32) float32 {
	if !(v > 0) {
		return 0
	}
	return min(v, maxItemSize)
}

// clampEstimate returns an estimated size: like clampSize, but at least
// 1 px, so that a list whose estimates are all zero does not take every
// item as visible before measuring them.
func clampEstimate(v float32) float32 {
	if !(v >= 1) {
		return 1
	}
	return min(v, maxItemSize)
}

// len returns the number of items.
func (x *sizeIndex) len() int { return len(x.size) }

// at returns the size of item i.
func (x *sizeIndex) at(i int) float32 { return x.size[i] }

// resize sets the number of items to n: the sizes of the items that remain
// are kept, new items take est(i). With reset, every item takes est(i).
func (x *sizeIndex) resize(n int, est func(int) float32, reset bool) {
	old := len(x.size)
	if reset {
		old = 0
	}
	if cap(x.size) >= n {
		x.size = x.size[:n]
	} else {
		s := make([]float32, n)
		copy(s, x.size[:min(old, len(x.size))])
		x.size = s
	}
	for i := min(old, n); i < n; i++ {
		x.size[i] = clampEstimate(est(i))
	}
	x.rebuild()
}

// rebuild computes the tree from the sizes in O(n).
func (x *sizeIndex) rebuild() {
	n := len(x.size)
	if cap(x.tree) >= n+1 {
		x.tree = x.tree[:n+1]
	} else {
		x.tree = make([]float64, n+1)
	}
	x.tree[0] = 0
	for k := 1; k <= n; k++ {
		x.tree[k] = float64(x.size[k-1])
	}
	for k := 1; k <= n; k++ {
		if j := k + k&-k; j <= n {
			x.tree[j] += x.tree[k]
		}
	}
	x.step = 0
	if n > 0 {
		x.step = 1
		for x.step*2 <= n {
			x.step *= 2
		}
	}
}

// set changes the size of item i to v.
func (x *sizeIndex) set(i int, v float32) {
	d := float64(v) - float64(x.size[i])
	x.size[i] = v
	if d == 0 {
		return
	}
	for k := i + 1; k < len(x.tree); k += k & -k {
		x.tree[k] += d
	}
}

// offset returns the sum of the sizes of the items before i (0 <= i <= n).
func (x *sizeIndex) offset(i int) float64 {
	var s float64
	for k := i; k > 0; k -= k & -k {
		s += x.tree[k]
	}
	return s
}

// total returns the sum of all sizes.
func (x *sizeIndex) total() float64 { return x.offset(len(x.size)) }

// before returns the number of leading items that end before y: the
// index of the first item whose end is at y or after it (n when every
// item ends before y). Sizes are never negative, so the ends grow with the
// index and the tree can be descended.
func (x *sizeIndex) before(y float64) int { return x.descend(y, false) }

// upTo returns the number of leading items that end at y or before it:
// the index of the first item whose end is after y.
func (x *sizeIndex) upTo(y float64) int { return x.descend(y, true) }

// descend returns the number of leading items whose end is below y (or
// at most y, with orEqual).
func (x *sizeIndex) descend(y float64, orEqual bool) int {
	pos := 0
	rem := y
	n := len(x.size)
	for step := x.step; step > 0; step >>= 1 {
		if k := pos + step; k <= n && (x.tree[k] < rem || orEqual && x.tree[k] == rem) {
			pos = k
			rem -= x.tree[k]
		}
	}
	return pos
}
