package style

import (
	"testing"

	"github.com/kuronosu/tela/internal/text"
)

// TestChUnitMatchesInter checks that ChPerEm approximates the advance of
// "0" in the embedded Inter, which is what the CSS ch unit means (within a
// few percent: the unit is documented as approximate).
func TestChUnitMatchesInter(t *testing.T) {
	c := text.NewDefaultCollection()
	p := &text.Paragraph{Text: "0", Default: text.TextStyle{Families: []string{"Inter"}, Size: 100}}
	l := text.NewLayout(c, p)
	l.BreakLines(1000)
	got := l.MaxContentWidth() / 100
	t.Logf("advance of 0 in Inter: %.4f em", got)
	if d := got - ChPerEm; d > 0.03 || d < -0.03 {
		t.Errorf("the advance of \"0\" in Inter is %.3f em, ChPerEm = %v", got, ChPerEm)
	}
}
