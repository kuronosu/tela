// Package editdist is the Levenshtein edit distance used by the "did you
// mean" suggestions of the layers above it (core, semantics, aria,
// telatest). It imports nothing; each caller keeps its own threshold.
package editdist

// Distance is the Levenshtein distance of a and b, counted in bytes.
func Distance(a, b string) int { return levenshtein([]byte(a), []byte(b)) }

// RuneDistance is the Levenshtein distance of a and b, counted in runes.
func RuneDistance(a, b string) int { return levenshtein([]rune(a), []rune(b)) }

func levenshtein[E byte | rune](a, b []E) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
