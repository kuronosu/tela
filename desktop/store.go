package desktop

import (
	"bytes"
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/kuronosu/tela/internal/core"
)

// Store is a small key-value store kept in a JSON file: an app's settings
// ([Settings]) or any other small state. Values are anything encoding/json
// encodes; reads decode them into the caller's type. Writes are kept in
// memory and written to the file shortly after (coalesced: a slider that
// sets a value every frame writes a few times a second), atomically (a
// temporary file renamed over the old one, so a crash never leaves half a
// file), and when the app quits. A Store is safe for use from any
// goroutine.
//
// The file is read once, when the store opens; changes made to it by other
// programs while the app runs are not seen, and are overwritten by the
// next write.
type Store struct {
	path  string
	delay time.Duration

	mu       sync.Mutex
	vals     map[string]json.RawMessage
	dirty    bool
	timer    *time.Timer
	corrupt  bool            // the file did not parse: moved aside at the first write
	unread   bool            // the file exists but could not be read: not overwritten
	touched  map[string]bool // keys changed while unread (they win over the file's)
	err      error           // the last load or write error
	watchers []*storeWatcher

	wmu sync.Mutex // serializes the writes of the file
}

type storeWatcher struct {
	key     string
	f       func(key string)
	stopped atomic.Bool
}

// writeDelay is how long a change waits before it is written, so that
// changes in a burst are written once.
const writeDelay = 100 * time.Millisecond

// The open stores by path: opening a path twice gives the same Store, so
// two parts of an app never overwrite each other's values.
var stores struct {
	mu     sync.Mutex
	byPath map[string]*Store
}

func init() {
	// Changes still in memory are written when tela.Run returns.
	core.OnRunEnd(func() { _ = FlushAll() })
}

// OpenStore opens the store kept in the JSON file at path (created at the
// first write, with its folder). Opening the same path again returns the
// same Store. A missing file is an empty store. A file that cannot be
// read or parsed gives an error, but the store is still returned and
// usable: it starts empty, and a file that did not parse is renamed to
// path + ".corrupt" before the first write replaces it, so nothing the
// user had is lost. A file that exists but cannot be read (permissions, a
// sharing violation, an I/O error) is never overwritten: the store works
// in memory, each write tries to read the file again (and merges it with
// the changes made meanwhile once it can), and [Store.Flush] returns the
// error until then.
func OpenStore(path string) (*Store, error) {
	if path == "" {
		panic("desktop.OpenStore(\"\"): give the path of the JSON file, such as filepath.Join(dir, \"recent.json\") with dir from desktop.StateDir()")
	}
	abs, err := filepath.Abs(path)
	if err == nil {
		path = abs
	}
	stores.mu.Lock()
	defer stores.mu.Unlock()
	if s := stores.byPath[path]; s != nil {
		return s, s.loadErr()
	}
	s := &Store{path: path, delay: writeDelay, vals: map[string]json.RawMessage{}}
	s.load()
	if stores.byPath == nil {
		stores.byPath = map[string]*Store{}
	}
	stores.byPath[path] = s
	return s, s.loadErr()
}

// loadErr returns the error of loading the file, if any.
func (s *Store) loadErr() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// load reads the file.
func (s *Store) load() {
	data, err := os.ReadFile(s.path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return
	case err != nil:
		s.unread = true
		s.err = fmt.Errorf("desktop: cannot read the store %s: %w; it starts empty and the file is not overwritten until it can be read", s.path, err)
		return
	}
	vals, err := parseStore(data)
	if err != nil {
		s.corrupt = true
		s.err = fmt.Errorf("desktop: the store %s is not a JSON object (%v); it starts empty and the file is kept as %s.corrupt", s.path, err, filepath.Base(s.path))
		return
	}
	maps.Copy(s.vals, vals)
}

// parseStore decodes the content of a store file. The values are compacted,
// so that they compare equal to what Set encodes (the file is indented).
func parseStore(data []byte) (map[string]json.RawMessage, error) {
	if len(bytes.TrimSpace(data)) == 0 {
		return nil, nil
	}
	var vals map[string]json.RawMessage
	if err := json.Unmarshal(data, &vals); err != nil {
		return nil, err
	}
	for k, v := range vals {
		if v == nil {
			delete(vals, k)
			continue
		}
		var b bytes.Buffer
		if json.Compact(&b, v) == nil {
			vals[k] = b.Bytes()
		}
	}
	return vals, nil
}

// retryLoad reads again a file that could not be read. It returns nil when
// the file can be written now: it was read (its keys that were not changed
// since are merged in), or it is gone, or it does not parse (then it is
// kept as .corrupt at the write). Called with s.mu held.
func (s *Store) retryLoad() error {
	data, err := os.ReadFile(s.path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("desktop: cannot write the store %s: the file exists but cannot be read (%w), so it is not overwritten", s.path, err)
	}
	if err == nil {
		vals, perr := parseStore(data)
		if perr != nil {
			s.corrupt = true
		}
		for k, v := range vals {
			if !s.touched[k] {
				s.vals[k] = v
			}
		}
	}
	s.unread, s.touched = false, nil
	return nil
}

// Settings returns the app's settings store, the file settings.json in
// [ConfigDir]. It never returns nil: when the folder cannot be found, the
// store lives in memory for this run, and when the file cannot be read or
// parsed it starts empty (see [OpenStore]); [Store.Err] says why.
//
//	var s struct{ Theme string; FontSize int }
//	desktop.Settings().Get("editor", &s)
//
// Components read settings with [UseSetting], which also builds them
// again when the value changes.
func Settings() *Store {
	dir, err := dirPath(dirConfig)
	if err != nil {
		return memoryStore("settings", err)
	}
	s, _ := OpenStore(filepath.Join(dir, "settings.json"))
	return s
}

// memoryStore returns the store for name that lives only in memory,
// because its file has no folder (err).
func memoryStore(name string, err error) *Store {
	key := "\x00memory:" + name
	stores.mu.Lock()
	defer stores.mu.Unlock()
	if s := stores.byPath[key]; s != nil {
		return s
	}
	s := &Store{delay: writeDelay, vals: map[string]json.RawMessage{}, err: err}
	if stores.byPath == nil {
		stores.byPath = map[string]*Store{}
	}
	stores.byPath[key] = s
	return s
}

// Path returns the file the store is kept in ("" for a store in memory).
func (s *Store) Path() string { return s.path }

// Err returns the error of the last failed read or write of the file,
// nil when the last one succeeded.
func (s *Store) Err() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.err
}

// Get decodes the value of key into ptr, a non-nil pointer (as
// json.Unmarshal does), and reports whether it did. A missing key, or a
// value that does not decode into ptr's type, leaves *ptr as it was and
// returns false, so a default set before the call stays:
//
//	size := 14 // the default
//	desktop.Settings().Get("fontSize", &size)
//
// The stored value is decoded over a copy of *ptr: a struct field the
// stored value does not have keeps the one *ptr had, while slices and
// maps the stored value has are replaced whole (a key removed from a
// stored map does not come back from the default). *ptr (a default's
// slice or map included) is never modified in place, and is assigned
// only when the call returns true.
func (s *Store) Get(key string, ptr any) bool {
	rv := reflect.ValueOf(ptr)
	if rv.Kind() != reflect.Pointer || rv.IsNil() {
		panic(fmt.Sprintf("desktop: Store.Get(%q, %T): pass a pointer to the variable to fill, such as Get(%q, &value)", key, ptr, key))
	}
	s.mu.Lock()
	raw, ok := s.vals[key]
	s.mu.Unlock()
	if !ok {
		return false
	}
	// Decode into a deep copy, so a failed decode leaves *ptr untouched and
	// a successful one never writes into memory the caller's value shares.
	v := reflect.New(rv.Elem().Type())
	deepCopy(v.Elem(), rv.Elem(), map[ptrKey]reflect.Value{})
	dropMaps(v.Elem(), raw)
	if err := json.Unmarshal(raw, v.Interface()); err != nil {
		return false
	}
	rv.Elem().Set(v.Elem())
	return true
}

var (
	jsonUnmarshaler = reflect.TypeFor[json.Unmarshaler]()
	textUnmarshaler = reflect.TypeFor[encoding.TextUnmarshaler]()
)

// dropMaps empties the maps of v (a copy of the default) that raw, the JSON
// about to be decoded over v, provides, so that decoding replaces them
// instead of adding entries to the default's. Struct fields raw does not
// have are left alone.
func dropMaps(v reflect.Value, raw json.RawMessage) {
	if v.CanAddr() {
		if pt := v.Addr().Type(); pt.Implements(jsonUnmarshaler) || pt.Implements(textUnmarshaler) {
			return // decodes itself
		}
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return
	}
	switch v.Kind() {
	case reflect.Map:
		if raw[0] == '{' {
			v.SetZero()
		}
	case reflect.Pointer:
		if !v.IsNil() {
			dropMaps(v.Elem(), raw)
		}
	case reflect.Array:
		var items []json.RawMessage
		if json.Unmarshal(raw, &items) == nil {
			for i := 0; i < v.Len() && i < len(items); i++ {
				dropMaps(v.Index(i), items[i])
			}
		}
	case reflect.Struct:
		var fields map[string]json.RawMessage
		if json.Unmarshal(raw, &fields) != nil {
			return
		}
		t := v.Type()
		for i := range t.NumField() {
			sf := t.Field(i)
			tag, hasTag := sf.Tag.Lookup("json")
			name, _, _ := strings.Cut(tag, ",")
			if tag == "-" {
				continue
			}
			if sf.Anonymous && name == "" {
				// An embedded struct's fields are in the same object.
				if sf.IsExported() || sf.Type.Kind() == reflect.Struct || sf.Type.Kind() == reflect.Pointer {
					dropMaps(v.Field(i), raw)
				}
				continue
			}
			if !sf.IsExported() {
				continue
			}
			if !hasTag || name == "" {
				name = sf.Name
			}
			fr, ok := fields[name]
			if !ok {
				for k, x := range fields {
					if strings.EqualFold(k, name) {
						fr, ok = x, true
						break
					}
				}
			}
			if ok {
				dropMaps(v.Field(i), fr)
			}
		}
	}
}

type ptrKey struct {
	p uintptr
	t reflect.Type
}

// deepCopy sets dst (settable) to a copy of src that shares no slice, map
// or pointer target with it. Unexported fields are copied shallowly:
// encoding/json does not touch them.
func deepCopy(dst, src reflect.Value, seen map[ptrKey]reflect.Value) {
	switch src.Kind() {
	case reflect.Pointer:
		if src.IsNil() {
			dst.Set(src)
			return
		}
		k := ptrKey{src.Pointer(), src.Type()}
		if c, ok := seen[k]; ok {
			dst.Set(c)
			return
		}
		c := reflect.New(src.Type().Elem())
		seen[k] = c
		deepCopy(c.Elem(), src.Elem(), seen)
		dst.Set(c)
	case reflect.Interface:
		if src.IsNil() {
			dst.Set(src)
			return
		}
		c := reflect.New(src.Elem().Type()).Elem()
		deepCopy(c, src.Elem(), seen)
		dst.Set(c)
	case reflect.Slice:
		if src.IsNil() {
			dst.Set(src)
			return
		}
		c := reflect.MakeSlice(src.Type(), src.Len(), src.Cap())
		for i := range src.Len() {
			deepCopy(c.Index(i), src.Index(i), seen)
		}
		dst.Set(c)
	case reflect.Array:
		dst.Set(src)
		for i := range src.Len() {
			deepCopy(dst.Index(i), src.Index(i), seen)
		}
	case reflect.Map:
		if src.IsNil() {
			dst.Set(src)
			return
		}
		c := reflect.MakeMapWithSize(src.Type(), src.Len())
		for it := src.MapRange(); it.Next(); {
			e := reflect.New(src.Type().Elem()).Elem()
			deepCopy(e, it.Value(), seen)
			c.SetMapIndex(it.Key(), e)
		}
		dst.Set(c)
	case reflect.Struct:
		dst.Set(src) // unexported fields
		for i := range src.NumField() {
			if f := dst.Field(i); f.CanSet() {
				deepCopy(f, src.Field(i), seen)
			}
		}
	default:
		dst.Set(src)
	}
}

// Has reports whether key has a value.
func (s *Store) Has(key string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.vals[key]
	return ok
}

// Keys returns the keys that have a value, sorted.
func (s *Store) Keys() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	keys := make([]string, 0, len(s.vals))
	for k := range s.vals {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}

// Set stores v under key. It returns an error, and changes nothing, when
// v cannot be encoded as JSON (a func, a channel). Setting a value equal
// to the stored one (the same JSON) does nothing; otherwise the watchers
// of key run, and the file is written shortly after.
func (s *Store) Set(key string, v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("desktop: Store.Set(%q): %w", key, err)
	}
	s.change(key, raw)
	return nil
}

// Delete removes key and its value; its watchers run.
func (s *Store) Delete(key string) { s.change(key, nil) }

// change sets key to raw (nil deletes it) and, when that changed it, runs
// the watchers and schedules the write.
func (s *Store) change(key string, raw json.RawMessage) {
	s.mu.Lock()
	old, had := s.vals[key]
	if (raw == nil && !had) || (raw != nil && had && bytes.Equal(old, raw)) {
		s.mu.Unlock()
		return
	}
	if raw == nil {
		delete(s.vals, key)
	} else {
		s.vals[key] = raw
	}
	if s.unread {
		if s.touched == nil {
			s.touched = map[string]bool{}
		}
		s.touched[key] = true
	}
	s.dirty = true
	if s.timer == nil && s.path != "" {
		s.timer = time.AfterFunc(s.delay, func() { _ = s.Flush() })
	}
	var ws []*storeWatcher
	for _, w := range s.watchers {
		if w.key == "" || w.key == key {
			ws = append(ws, w)
		}
	}
	s.mu.Unlock()
	for _, w := range ws {
		if !w.stopped.Load() {
			w.f(key)
		}
	}
}

// Watch calls f after each change of key's value (Set with a different
// value, Delete), or of any key when key is "", until stop is called. f
// runs on the goroutine that made the change, after it, without the
// store's lock: it may read the store and set values. In components, use
// [UseSetting], which builds them again on change.
func (s *Store) Watch(key string, f func(key string)) (stop func()) {
	if f == nil {
		panic("desktop: Store.Watch(key, nil): pass the function to call after a change")
	}
	w := &storeWatcher{key: key, f: f}
	s.mu.Lock()
	s.watchers = append(s.watchers, w)
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.stopped.Store(true)
		s.watchers = slices.DeleteFunc(s.watchers, func(x *storeWatcher) bool { return x == w })
	}
}

// Flush writes the changes still in memory now. It is called by itself
// shortly after a change and when tela.Run returns; call it before a
// program that does not use tela.Run exits. A store in memory does
// nothing.
func (s *Store) Flush() error {
	s.wmu.Lock()
	defer s.wmu.Unlock()
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if !s.dirty || s.path == "" {
		s.mu.Unlock()
		return nil
	}
	if s.unread {
		if err := s.retryLoad(); err != nil {
			s.err = err
			s.mu.Unlock()
			return err // dirty stays: written by the next change or Flush
		}
	}
	data, err := json.MarshalIndent(s.vals, "", "  ")
	corrupt := s.corrupt
	s.dirty = false
	s.mu.Unlock()
	if err == nil {
		err = writeStore(s.path, append(data, '\n'), corrupt)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if err != nil {
		s.dirty = true // written by the next change or Flush
		s.err = fmt.Errorf("desktop: cannot write the store %s: %w", s.path, err)
		return s.err
	}
	s.corrupt, s.err = false, nil
	return nil
}

// writeStore writes data to path atomically: a temporary file in the same
// folder, synced, then renamed over path (os.Rename replaces the file on
// every OS, MoveFileEx with MOVEFILE_REPLACE_EXISTING on Windows). A
// corrupt file is renamed to path.corrupt first.
func writeStore(path string, data []byte, corrupt bool) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if corrupt {
		if err := os.Rename(path, path+".corrupt"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, err = f.Write(data)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, path)
	}
	if err != nil {
		os.Remove(tmp)
	}
	return err
}

// FlushAll writes the changes of every open store ([Store.Flush]) and
// returns the first error. tela.Run calls it when it returns.
func FlushAll() error {
	stores.mu.Lock()
	all := make([]*Store, 0, len(stores.byPath))
	for _, s := range stores.byPath {
		all = append(all, s)
	}
	stores.mu.Unlock()
	var first error
	for _, s := range all {
		if err := s.Flush(); err != nil && first == nil {
			first = err
		}
	}
	return first
}
