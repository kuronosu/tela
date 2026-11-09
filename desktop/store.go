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
	"runtime"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
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
// Changes other programs make to the file are seen while the store has
// watchers ([Store.Watch], [UseSetting]): the file is read again and
// merged key by key, and the watchers of the keys that changed run. For
// a key both sides changed, this process's change not yet written wins
// (it is written a moment later); otherwise the file's value wins. A
// write reads the file first and merges it the same way, so it never
// drops a key another program wrote.
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
	pending  map[string]bool // keys changed since the last write (storewatch.go)
	fw       *fileWatch      // the watch of the file while there are watchers
	opening  bool            // a watch is being opened (openWatch, without mu)
	stopT    *time.Timer     // the stop of the watch after the last watcher (stopWatch)
	read     fileStamp       // the file as the last reload merged it (under wmu)

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
// stored value does not have keeps the one *ptr had, while slices, arrays
// and maps the stored value has are replaced whole, and their elements
// start empty (a key removed from a stored map does not come back from the
// default, nor does a field an element omits through "omitempty").
// *ptr (a default's slice or map included) is never modified in place, and
// is assigned only when the call returns true.
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
	dropCollections(v.Elem(), raw)
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

// dropCollections empties the maps, slices and arrays of v (a copy of the
// default) that raw, the JSON about to be decoded over v, provides, so that
// decoding replaces them: encoding/json would otherwise add entries to a
// map and decode each slice or array element over the default's element at
// the same index, so a field the stored element omits (omitempty) would
// come back from the default. Struct fields raw does not have are left
// alone.
func dropCollections(v reflect.Value, raw json.RawMessage) {
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
			dropCollections(v.Elem(), raw)
		}
	case reflect.Slice, reflect.Array:
		if raw[0] == '[' {
			v.SetZero()
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
					dropCollections(v.Field(i), raw)
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
				dropCollections(v.Field(i), fr)
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
	if s.pending == nil {
		s.pending = map[string]bool{}
	}
	s.pending[key] = true
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
// value, Delete, another program's write to the file), or of any key when
// key is "", until stop is called. f runs on the goroutine that made the
// change (the store's watcher goroutine for another program's), after it,
// without the store's lock: it may read the store and set values. In components, use
// [UseSetting], which builds them again on change.
func (s *Store) Watch(key string, f func(key string)) (stop func()) {
	if f == nil {
		panic("desktop: Store.Watch(key, nil): pass the function to call after a change")
	}
	w := &storeWatcher{key: key, f: f}
	s.mu.Lock()
	s.watchers = append(s.watchers, w)
	start := s.claimWatch() // storewatch.go: changes of other programs
	s.mu.Unlock()
	if start {
		go s.openWatch()
	}
	return func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		w.stopped.Store(true)
		s.watchers = slices.DeleteFunc(s.watchers, func(x *storeWatcher) bool { return x == w })
		s.stopWatch()
	}
}

// Flush writes the changes still in memory now. It is called by itself
// shortly after a change and when tela.Run returns; call it before a
// program that does not use tela.Run exits. A store in memory does
// nothing.
func (s *Store) Flush() error {
	s.wmu.Lock()
	notify := []func(){s.reload()} // merge what other programs wrote since (storewatch.go)
	var err error
	for try := 0; ; try++ {
		// Another program that wrote the file after the reload, before
		// the replace, is read and merged once more (twice at most: a
		// file rewritten all the time is replaced at the third try).
		var replaced *replacedFile
		replaced, err = s.flushLocked(try < maxMergeRetries)
		if !errors.Is(err, errFileChanged) {
			break
		}
		if replaced != nil {
			// The write replaced it already (an exchange): merge the
			// version it replaced, and write again.
			notify = append(notify, s.merge(replaced.data, replaced.written))
		} else {
			notify = append(notify, s.reload())
		}
	}
	s.wmu.Unlock()
	for _, f := range notify {
		f()
	}
	return err
}

// maxMergeRetries is how many times a write reads the file again when
// another program changed it during the write.
const maxMergeRetries = 2

// errFileChanged is flushLocked's report that the file changed since the
// last reload, before the replace: nothing was written, or (with a
// replacedFile) the version another program wrote was replaced and is to
// be merged and written again.
var errFileChanged = errors.New("desktop: the store's file changed during the write")

// replacedFile is a version of the file another program wrote after the
// last reload, which a write replaced through an exchange (swapIn).
type replacedFile struct {
	data    []byte
	written fileStamp // what the write put in place
}

// flushLocked is Flush's write, with s.wmu held. With check, the write
// never replaces a version of the file other than the one the last
// reload merged (s.read) without saying so: it returns errFileChanged, with
// the version it replaced when it could tell only after the replace.
func (s *Store) flushLocked(check bool) (*replacedFile, error) {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if !s.dirty || s.path == "" {
		s.mu.Unlock()
		return nil, nil
	}
	if s.unread {
		if err := s.retryLoad(); err != nil {
			s.err = err
			s.mu.Unlock()
			return nil, err // dirty stays: written by the next change or Flush
		}
	}
	data, err := json.MarshalIndent(s.vals, "", "  ")
	corrupt := s.corrupt
	s.dirty = false
	written := s.pending
	s.pending = nil
	s.mu.Unlock()
	var replaced *replacedFile
	if err == nil {
		data = append(data, '\n')
		read := fileStamp{}
		if check && !corrupt {
			read = s.read
		}
		var old []byte
		old, err = writeStore(s.path, data, corrupt, read)
		if old != nil {
			own := stampOf(data)
			own.own = true
			replaced = &replacedFile{data: old, written: own}
		}
	}
	s.mu.Lock()
	if err != nil {
		s.dirty = true // written by the next change or Flush
		for k := range written {
			if s.pending == nil {
				s.pending = map[string]bool{}
			}
			s.pending[k] = true
		}
		if errors.Is(err, errFileChanged) {
			s.mu.Unlock()
			return replaced, err // Flush merges the file and writes again
		}
		s.err = fmt.Errorf("desktop: cannot write the store %s: %w", s.path, err)
		err = s.err
		s.mu.Unlock()
		return nil, err
	}
	s.corrupt, s.err = false, nil
	start := s.claimWatch() // the folder exists now
	s.mu.Unlock()
	if start {
		go s.openWatch()
	}
	return nil, nil
}

// writeStore writes data to path atomically: a temporary file in the same
// folder, synced, then put in place of path (renamed over it: os.Rename
// replaces the file on every OS, MoveFileEx with MOVEFILE_REPLACE_EXISTING
// on Windows). A corrupt file is renamed to path.corrupt first.
//
// When read is a stamp taken (the version of the file the last reload
// merged), the temporary file replaces only that version (swapIn): when
// another program wrote the file meanwhile, writeStore returns
// errFileChanged, and also the content of the version it replaced when it
// could tell only by replacing it (an exchange), which the caller merges
// and writes again.
func writeStore(path string, data []byte, corrupt bool, read fileStamp) (replaced []byte, err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if corrupt {
		if err := rename(path, path+".corrupt"); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err
		}
	}
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".*.tmp")
	if err != nil {
		return nil, err
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
		hook(&beforeRename, path)
	}
	swapped := false
	if err == nil {
		if read.ok {
			swapped, replaced, err = swapIn(tmp, path, read)
		} else {
			err = rename(tmp, path)
		}
	}
	if err != nil || swapped {
		os.Remove(tmp) // the temporary file, or the version an exchange replaced
	}
	return replaced, err
}

// swapIn puts the file tmp in place of path, unless path is no longer the
// version read (ADR-262). On Linux and macOS the replace is atomic and
// tells what it replaced: tmp is exchanged with path (renameat2
// RENAME_EXCHANGE, renamex_np RENAME_SWAP), then the version now at tmp is
// read and compared with read; when it is another one, its content is
// returned with errFileChanged (the caller merges it and writes again). A
// file that did not exist is created only if it still does not
// (RENAME_NOREPLACE, RENAME_EXCL), else errFileChanged with nothing
// written. Where the OS or the file system cannot (Windows; NFS, FAT and
// other file systems without the flags; kernels before 3.15), the file is
// compared right before a plain rename, which leaves the moment between
// the two open. swapped reports an exchange: tmp then holds the version
// replaced, for the caller to remove.
func swapIn(tmp, path string, read fileStamp) (swapped bool, replaced []byte, err error) {
	if read.exists {
		hook(&atReplace, path)
		err := exchangeFiles(tmp, path)
		switch {
		case err == nil:
			old, rerr := os.ReadFile(tmp)
			if rerr != nil || read.same(stampOf(old)) {
				// The version read (or one that cannot be read, which
				// nothing could merge): written.
				return true, nil, nil
			}
			return true, old, errFileChanged
		case errors.Is(err, fs.ErrNotExist):
			return false, nil, errFileChanged // removed meanwhile: nothing written
		case !errors.Is(err, errors.ErrUnsupported):
			return false, nil, err
		}
	} else {
		hook(&atReplace, path)
		err := renameNoReplace(tmp, path)
		switch {
		case err == nil:
			return false, nil, nil
		case errors.Is(err, fs.ErrExist):
			return false, nil, errFileChanged // created meanwhile: nothing written
		case !errors.Is(err, errors.ErrUnsupported):
			return false, nil, err
		}
	}
	if changedSince(path, read) {
		return false, nil, errFileChanged
	}
	hook(&atReplace, path)
	return false, nil, rename(tmp, path)
}

// beforeRename, when set, runs in writeStore before its check, with the
// store's path (tests: another program's write at the edge of this one's).
// atReplace runs right before the call that replaces the file, after any
// check (tests: another program's write in the moment that only an
// exchange closes); it may run twice when an exchange is unsupported.
var beforeRename, atReplace atomic.Pointer[func(path string)]

// hook runs the test hook h with path, if set.
func hook(h *atomic.Pointer[func(path string)], path string) {
	if f := h.Load(); f != nil {
		(*f)(path)
	}
}

// rename is os.Rename, retried for a moment on Windows, where replacing or
// moving a file fails with ERROR_ACCESS_DENIED or ERROR_SHARING_VIOLATION
// while another program has it open without sharing its deletion: an
// antivirus scanning the file just written, the search indexer, a reader
// of the old file. cmd/go retries the same way (its robustio package).
func rename(from, to string) error {
	err := os.Rename(from, to)
	if runtime.GOOS != "windows" {
		return err
	}
	for deadline := time.Now().Add(time.Second); err != nil && busyOnWindows(err) && time.Now().Before(deadline); {
		time.Sleep(10 * time.Millisecond)
		err = os.Rename(from, to)
	}
	return err
}

// busyOnWindows reports ERROR_ACCESS_DENIED (5) and ERROR_SHARING_VIOLATION
// (32), Windows' errors for a file another program holds; the caller checks
// the OS.
func busyOnWindows(err error) bool {
	var errno syscall.Errno
	return errors.As(err, &errno) && (errno == 5 || errno == 32)
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
