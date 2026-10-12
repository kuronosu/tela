package style

// This file computes "did you mean" suggestions: the closest class of the
// vocabulary by Damerau-Levenshtein distance (optimal string alignment).

// closest returns the entry of vocab closest to name, or "" when nothing
// is close enough. Ties prefer an entry with the same first segment (the
// same Tailwind namespace), then the same length (a typo that substitutes
// a character is likelier than one that drops it, so "rounded-xxl" gives
// "rounded-2xl" and not "rounded-xl"), then the longer shared prefix, then
// the earlier entry (vocab is sorted).
func closest(name string, vocab []string) string {
	if name == "" {
		return ""
	}
	limit := min(2+len(name)/6, 4)
	best, bestDist := "", limit+1
	var bestKey [3]int
	var prev2, prev, cur []int
	root := firstSegment(name)
	for _, cand := range vocab {
		if cand == name {
			return name
		}
		if d := len(cand) - len(name); d > bestDist || -d > bestDist {
			continue
		}
		dist := osaDistance(name, cand, bestDist, &prev2, &prev, &cur)
		if dist > bestDist {
			continue
		}
		key := [3]int{0, 0, -commonPrefix(name, cand)}
		if firstSegment(cand) != root {
			key[0] = 1
		}
		if len(cand) != len(name) {
			key[1] = 1
		}
		if dist < bestDist || (dist == bestDist && less3(key, bestKey)) {
			best, bestDist, bestKey = cand, dist, key
		}
	}
	return best
}

func less3(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}

func firstSegment(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] == '-' {
			return s[:i]
		}
	}
	return s
}

func commonPrefix(a, b string) int {
	n := min(len(a), len(b))
	i := 0
	for i < n && a[i] == b[i] {
		i++
	}
	return i
}

// osaDistance returns the optimal-string-alignment distance of a and b, or
// a value above limit when it exceeds limit. The row buffers are reused
// between calls.
func osaDistance(a, b string, limit int, p2, p1, c0 *[]int) int {
	n := len(b) + 1
	for _, r := range []*[]int{p2, p1, c0} {
		if cap(*r) < n {
			*r = make([]int, n)
		}
		*r = (*r)[:n]
	}
	prev2, prev, cur := *p2, *p1, *c0
	for j := range prev {
		prev[j] = j
	}
	prevMin := 0
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		rowMin := cur[0]
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				v = min(v, prev2[j-2]+1)
			}
			cur[j] = v
			rowMin = min(rowMin, v)
		}
		// A transposition looks two rows back, so a row can be lower than
		// the one before it; stop only when two rows in a row are over.
		if rowMin > limit && prevMin > limit {
			return limit + 1
		}
		prevMin = rowMin
		prev2, prev, cur = prev, cur, prev2
	}
	return prev[len(b)]
}
