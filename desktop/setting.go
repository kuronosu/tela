package desktop

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/kuronosu/tela"
)

// UseSetting returns the value of key in the app's settings ([Settings]),
// or def when it has none, and a setter that stores a new value: the
// useState of a setting. The component builds again when the value
// changes, whoever changed it (this component, another one with the same
// key, Settings().Set from any goroutine), and the store writes the file
// shortly after a change.
//
//	func (Editor) Build() tela.Node {
//	    size, setSize := desktop.UseSetting("editor.fontSize", 14)
//	    return Div(
//	        Button(OnClick(func() { setSize.Update(func(n int) int { return n + 1 }) }), Text("A+")),
//	        Textarea(Style(tela.FontSize(float32(size)))),
//	    )
//	}
//
// T must encode as JSON. A stored value that does not decode into T (the
// setting changed type between versions of the app) reads as def. key
// must stay the same for the life of the component (give it another
// tela key to read another setting).
func UseSetting[T any](key string, def T) (T, tela.Setter[T]) {
	s := tela.UseOnce(Settings)
	return useStoreSetting("UseSetting", s, key, def)
}

// UseStoreSetting is [UseSetting] for a key of store s instead of the
// app's settings.
func UseStoreSetting[T any](s *Store, key string, def T) (T, tela.Setter[T]) {
	if s == nil {
		panic("desktop.UseStoreSetting: the store is nil; open it with desktop.OpenStore(path), or use desktop.UseSetting for the app's settings")
	}
	return useStoreSetting("UseStoreSetting", s, key, def)
}

// settingLink is the JSON of a setting as the component and the store last
// agreed on it, so that a change made on one side is copied to the other
// once (guarded by mu: the store's watchers run on any goroutine).
type settingLink struct {
	mu     sync.Mutex
	synced string
}

func useStoreSetting[T any](what string, s *Store, key string, def T) (T, tela.Setter[T]) {
	read := func() T {
		v := def
		s.Get(key, &v)
		return v
	}
	initial := tela.UseOnce(read)
	v, set := tela.UseState(initial)
	link := tela.UseOnce(func() *settingLink { return &settingLink{synced: encodeSetting(what, key, initial)} })
	enc := encodeSetting(what, key, v)

	// The component changed the value (set): store it.
	tela.UseEffectDo(func() {
		link.mu.Lock()
		changed := enc != link.synced
		link.synced = enc
		link.mu.Unlock()
		if changed {
			_ = s.Set(key, json.RawMessage(enc))
		}
	}, enc)

	// The store changed: show the new value.
	tela.UseMount(func() func() {
		sync := func(string) {
			nv := read()
			e := encodeSetting(what, key, nv)
			link.mu.Lock()
			changed := e != link.synced
			link.synced = e
			link.mu.Unlock()
			if changed {
				set.Set(nv)
			}
		}
		stop := s.Watch(key, sync)
		sync(key) // a change between the first build and now
		return stop
	})
	return v, set
}

// encodeSetting returns v as JSON, panicking with the fix when T cannot be
// encoded (a programmer error).
func encodeSetting[T any](what, key string, v T) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("desktop.%s(%q): the value %T cannot be stored as JSON (%v); store plain data: numbers, strings, bools, slices, maps and structs with exported fields", what, key, v, err))
	}
	return string(b)
}
