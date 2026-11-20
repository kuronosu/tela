package core

// HasElement reports whether element id is mounted in the tree, as
// Element does, without building its ElementInfo (which formats every
// hook's value). False while the instrumentation is off.
func (t *Tree) HasElement(id ElementID) bool {
	return t.ins != nil && t.ins.element(id) != nil
}
