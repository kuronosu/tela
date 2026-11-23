package core

import "testing"

// The lines AuditTexts reports are those of the box's final width, not the
// ones a measuring pass left in the paragraph.
func TestAuditTextsBreaksForTheFinalWidth(t *testing.T) {
	tr := newTestTree(t, 400, 100, div(Class("w-[380px] p-2"), Text("one two three four five six seven eight")), nil)
	tr.Frame()
	var para *Box
	var walk func(b *Box)
	walk = func(b *Box) {
		if b.mode == modeParagraph && b.para != nil && b.para.lay != nil {
			para = b
		}
		for _, k := range b.kids {
			walk(k)
		}
	}
	walk(tr.rootBox)
	if para == nil {
		t.Fatal("no paragraph")
	}
	// A measuring pass at the minimum content width leaves the lines broken
	// at one word each.
	para.para.lay.BreakLines(1)
	para.para.brokeAt = 1
	texts := tr.AuditTexts()
	if len(texts) != 1 || len(texts[0].Lines) != 1 {
		t.Fatalf("texts: %+v", texts)
	}
}
