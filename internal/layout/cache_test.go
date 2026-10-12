package layout

import "testing"

// Definite available space can be negative (a narrow box with large
// padding); its key must differ from a known dimension of the opposite
// sign and from the other kinds of space.
func TestCacheKeyKinds(t *testing.T) {
	keys := map[string]cacheKey{
		"known 10":      makeKey(&input{known: optSize{w: some(10)}, knownDefinite: boolSize{w: true}, avail: availSize{w: DefiniteSpace(-10)}}),
		"available -10": makeKey(&input{avail: availSize{w: DefiniteSpace(-10)}}),
		"available 10":  makeKey(&input{avail: availSize{w: DefiniteSpace(10)}}),
		"min-content":   makeKey(&input{avail: availSize{w: MinContentConstraint}}),
		"max-content":   makeKey(&input{avail: availSize{w: MaxContentConstraint}}),
	}
	for a, ka := range keys {
		for b, kb := range keys {
			if a != b && ka.sameRequest(&kb) {
				t.Errorf("%s and %s give the same cache key", a, b)
			}
		}
	}
}
