package aria

import (
	"slices"

	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/internal/core"
)

// keyList is what a rover and a collState share: the keys of the items in
// order with their index, the disabled ones, and an ElementRef per key.
type keyList[K comparable] struct {
	keys     []K
	index    map[K]int
	disabled map[K]struct{}
	all      bool // the whole list is disabled
	refs     map[K]tela.ElementRef
}

// setDisabled takes the disabled keys of a build (usually few).
func (l *keyList[K]) setDisabled(disabled []K) {
	if len(disabled) == 0 {
		l.disabled = nil
		return
	}
	l.disabled = make(map[K]struct{}, len(disabled))
	for _, k := range disabled {
		l.disabled[k] = struct{}{}
	}
}

// setKeys takes the keys of a build. The index is rebuilt, and the refs of
// the keys that went are dropped, only when the keys changed (or on the
// first call); it then returns the previous keys and true.
func (l *keyList[K]) setKeys(keys []K) (old []K, changed bool) {
	if l.index != nil && slices.Equal(l.keys, keys) {
		return nil, false
	}
	old = l.keys
	l.keys = slices.Clone(keys)
	l.index = make(map[K]int, len(keys))
	for i, k := range keys {
		l.index[k] = i
	}
	for k := range l.refs {
		if _, ok := l.index[k]; !ok {
			delete(l.refs, k)
		}
	}
	return old, true
}

func (l *keyList[K]) has(k K) bool {
	_, ok := l.index[k]
	return ok
}

// isDisabled reports whether the item is disabled, by its key or by the
// list.
func (l *keyList[K]) isDisabled(k K) bool {
	if l.all {
		return true
	}
	_, d := l.disabled[k]
	return d
}

func (l *keyList[K]) refFor(k K) tela.ElementRef {
	ref, ok := l.refs[k]
	if !ok {
		ref = core.NewElementRef()
		l.refs[k] = ref
	}
	return ref
}

func (l *keyList[K]) firstKey() (K, bool) {
	for _, k := range l.keys {
		if !l.isDisabled(k) {
			return k, true
		}
	}
	var zero K
	return zero, false
}

func (l *keyList[K]) lastKey() (K, bool) {
	for i := len(l.keys) - 1; i >= 0; i-- {
		if !l.isDisabled(l.keys[i]) {
			return l.keys[i], true
		}
	}
	var zero K
	return zero, false
}
