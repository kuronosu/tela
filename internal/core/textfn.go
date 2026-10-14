package core

// Phase-deferred text (ARCHITECTURE section 7, ADR-070). A TextFn node
// reads its content when the box phase assembles its paragraph, not when
// the component builds: the Values the function reads are tracked
// (obsText), and a change marks the text's parent for the box phase, which
// assembles the paragraph again and lays it out when the text changed. No
// component builds, so a clock or a progress label does not rebuild the
// component that holds it.

// TextFn returns a text node whose content is the result of fn, read in
// the box phase and again whenever a Value that fn read changes. It lays
// out like Text: inside paragraphs, next to other text and inline
// elements. fn runs on the UI thread and should only compute the string;
// a closure that captures state of the component is replaced by the new
// one every time the component builds.
func TextFn(fn func() string) Node {
	if fn == nil {
		panic("tela: TextFn(nil): pass a function that returns the text, such as TextFn(func() string { return time.Get() })")
	}
	return Node{&desc{kind: nodeText, textFn: fn}}
}

// syncText makes the text of text element el that of node description d,
// on mount and update. It reports whether the text changed.
func (t *Tree) syncText(el *Element, d *desc) bool {
	if d.textFn == nil {
		if el.tfn != nil {
			// A TextFn became a Text: it reads nothing any more.
			el.tfn, el.textStale = nil, false
			dropObserver(el.vobs)
		}
		changed := el.text != d.text
		el.text = d.text
		return changed
	}
	el.tfn = d.textFn
	return t.evalText(el)
}

// evalText evaluates the function of text element el, tracking the Values
// it reads, and reports whether the text changed.
func (t *Tree) evalText(el *Element) bool {
	mark := t.beginTrack()
	var s string
	func() {
		done := false
		defer func() {
			if !done { // fn panicked: keep the old subscriptions
				t.endTrack(mark)
			}
		}()
		s = el.tfn()
		done = true
	}()
	reads := t.endTrack(mark)
	t.observe(&el.vobs, obsText, el, nil, reads)
	el.textStale = false
	changed := s != el.text
	el.text = s
	return changed
}

// textOf returns the text of text element el for a paragraph being
// assembled: a TextFn whose Values changed is evaluated again first.
func (t *Tree) textOf(el *Element) string {
	if el.tfn != nil && el.textStale {
		t.evalText(el)
	}
	return el.text
}
