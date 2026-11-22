package lint

import (
	"bytes"
	"crypto/sha256"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Cache keeps, between runs, the packages that a run checks from source
// but does not lint: the packages of Options.Context that declare aliases
// and colors, with what they import of their module, and the dependencies
// that import a linted package. A run of tela lsp lints one package with
// the whole module as context, and in Tela's own repository that context is
// most of the framework, checked with bodies: about 1.2 s of a 1.8 s run.
// With a Cache, the runs after the first one check only what changed.
//
// A package is reused when nothing a fresh check reads differs: its go
// list entry (files, imports, the import map, the Go version), the content
// of each of its files (the overlay's, or the disk's), and the package each
// of its imports resolves to in this run, compared by identity. The last
// one makes invalidation transitive: a package whose dependency was checked
// again, or read from another generation of export data (export.go), is
// checked again too, since its types refer to the old dependency's
// objects. Nothing depends on the editor telling what changed: go list runs
// for every run, so an added, deleted or renamed file, a changed build
// constraint or import is seen, and a file edited on disk by another
// program is seen by its size and modification time, or its content.
//
// The packages being linted are never taken from the cache, nor stored in
// it: their syntax is the run's (the comments of the ignore directives, the
// overlay's text), and an editor lints them because they changed.
//
// A Cache may be used by several runs at a time. The zero value is not
// ready; use NewCache.
type Cache struct {
	mu     sync.Mutex
	epoch  *cacheEpoch
	stamps map[string]fileStamp // by file path
	reused int                  // packages taken from the cache, for tests
}

// NewCache returns an empty cache.
func NewCache() *Cache {
	return &Cache{epoch: newCacheEpoch(), stamps: map[string]fileStamp{}}
}

// cacheEpoch is the file set and the packages checked into it. The syntax
// of a package that was replaced stays in the file set (a run that took it
// before may still read its positions); when that garbage outgrows what is
// live, later runs start a new epoch, and the old one goes when its last
// run ends.
type cacheEpoch struct {
	fset *token.FileSet

	mu      sync.Mutex
	pkgs    map[string]*cachedPackage // by import path
	flights map[string]*flight        // the checks being made, by import path
	files   int                       // files of the packages in pkgs
	garbage int                       // files of the packages replaced
}

// minGarbage is the number of replaced files an epoch keeps before the
// garbage may make a new one.
const minGarbage = 2000

func newCacheEpoch() *cacheEpoch {
	return &cacheEpoch{fset: token.NewFileSet(), pkgs: map[string]*cachedPackage{}, flights: map[string]*flight{}}
}

// cachedPackage is a package as a run checked it.
type cachedPackage struct {
	key    string // packageKey
	bodies bool   // checked with function bodies and Info (a context package)
	c      *checked
	// imports is what each import of the package resolved to.
	imports map[string]*types.Package
}

// currentEpoch returns the epoch a new run checks into.
func (c *Cache) currentEpoch() *cacheEpoch {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.epoch
}

// flight is a check of a package that a run is making for the cache.
type flight struct {
	key     string
	bodies  bool
	imports map[string]*types.Package
	done    chan struct{}
	stored  bool // set before done is closed
}

// matches reports whether a package checked with key, bodies and imports
// serves a run that needs key, its imports and, for a target, bodies.
func matches(key string, bodies bool, imports map[string]*types.Package, wantKey string, target bool, wantImports map[string]*types.Package) bool {
	return key == wantKey && (bodies || !target) && sameImports(imports, wantImports)
}

// claim returns the cached package that serves the run, or else a flight:
// one of another run to wait for (own false), or the run's own, which it
// must land.
func (e *cacheEpoch) claim(path, key string, target bool, imports map[string]*types.Package) (*cachedPackage, *flight, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if p := e.pkgs[path]; p != nil && matches(p.key, p.bodies, p.imports, key, target, imports) {
		return p, nil, false
	}
	if f := e.flights[path]; f != nil && matches(f.key, f.bodies, f.imports, key, target, imports) {
		return nil, f, false
	}
	f := &flight{key: key, bodies: target, imports: imports, done: make(chan struct{})}
	if e.flights[path] == nil {
		e.flights[path] = f // else another check is in flight: this one is not waited for
	}
	return nil, f, true
}

// land stores the result of the run's own flight (unless it did not
// type-check at all) and wakes the runs waiting for it. It reports whether
// the epoch has grown more garbage than live syntax.
func (e *cacheEpoch) land(path string, f *flight, c *checked) (stale bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.flights[path] == f {
		delete(e.flights, path)
	}
	defer close(f.done)
	if c.pkg == nil {
		return false
	}
	f.stored = true
	if old := e.pkgs[path]; old != nil {
		e.files -= len(old.c.files)
		e.garbage += len(old.c.files)
	}
	e.pkgs[path] = &cachedPackage{key: f.key, bodies: f.bodies, c: c, imports: f.imports}
	e.files += len(c.files)
	return e.garbage > minGarbage && e.garbage > e.files
}

// retire makes later runs use a new epoch, if e is still the current one.
func (c *Cache) retire(e *cacheEpoch) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.epoch == e {
		c.epoch = newCacheEpoch()
	}
}

// fileStamp is what the cache knows of a file on disk: its size and
// modification time when it was read, and the hash of what was read.
type fileStamp struct {
	size int64
	mod  time.Time
	// trusted is false when the file was read less than stampSlack after
	// it was modified: a file system with coarse times (one or two
	// seconds: FAT, HFS+, some network file systems) can write it again
	// with the same size and time, so it is read again next time.
	trusted  bool
	sum      [sha256.Size]byte
	mentions bool // the text names ClassAliases or SchemeColor
}

// stampSlack is the coarsest modification time a file system is expected
// to keep (FAT's two seconds), the same caution gopls takes with file
// times.
const stampSlack = 2 * time.Second

// stamp returns the stamp of a file on disk, reading it only when its size
// or modification time changed since it was last read (or it was read too
// soon after a change to tell).
func (c *Cache) stamp(path string) (fileStamp, bool) {
	fi, err := os.Stat(path)
	if err != nil || !fi.Mode().IsRegular() {
		return fileStamp{}, false
	}
	c.mu.Lock()
	st, ok := c.stamps[path]
	c.mu.Unlock()
	if ok && st.trusted && st.size == fi.Size() && st.mod.Equal(fi.ModTime()) {
		return st, true
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return fileStamp{}, false
	}
	st = fileStamp{
		size: fi.Size(), mod: fi.ModTime(),
		trusted:  time.Since(fi.ModTime()) > stampSlack,
		sum:      sha256.Sum256(data),
		mentions: mentionsAliasText(data),
	}
	c.mu.Lock()
	c.stamps[path] = st
	c.mu.Unlock()
	return st, true
}

// mentionsAliasText reports whether a file's text names ClassAliases or
// SchemeColor, so that it may declare class aliases or theme colors.
func mentionsAliasText(data []byte) bool {
	return bytes.Contains(data, []byte("ClassAliases")) || bytes.Contains(data, []byte("SchemeColor"))
}

// packageKey is what decides whether a cached package is the one a fresh
// check of lp would make, but for its imports: the parts of its listing
// that the check reads, and the content of each file. It is "" when a file
// cannot be read, and the package is then not cached.
func (c *Cache) packageKey(lp *listPackage, overlay map[string][]byte, path func(name string) string) string {
	h := sha256.New()
	field := func(s string) {
		var n [8]byte
		for i, l := 0, len(s); i < 8; i, l = i+1, l>>8 {
			n[i] = byte(l)
		}
		h.Write(n[:])
		h.Write([]byte(s))
	}
	field(lp.ImportPath)
	field(lp.Dir)
	field(lp.Name)
	field(goVersion(lp))
	keys := make([]string, 0, len(lp.ImportMap))
	for k := range lp.ImportMap {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		field(k)
		field(lp.ImportMap[k])
	}
	for _, name := range lp.GoFiles {
		file := path(name)
		field(name)
		if src, ok := overlay[file]; ok {
			sum := sha256.Sum256(src)
			field("overlay")
			h.Write(sum[:])
			continue
		}
		st, ok := c.stamp(file)
		if !ok {
			return ""
		}
		field("disk")
		h.Write(st.sum[:])
	}
	return string(h.Sum(nil))
}

// useCache makes the loader check the packages it does not lint through
// c, into the file set of c's current epoch; fresh are the packages being
// linted. It is called before anything is parsed.
func (l *loader) useCache(c *Cache, fresh map[string]bool) {
	l.cache = c
	l.ep = c.currentEpoch()
	l.fset = l.ep.fset
	l.fresh = fresh
	l.cached = map[string]bool{}
	l.reused = map[string]bool{}
	l.kept = map[string]bool{}
}

// reusedPaths returns the packages this run took from the cache.
func (l *loader) reusedPaths() map[string]bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.reused
}

// checkCached returns the cached package of path when it is the one a
// check would make now, else checks it, and stores it when everything it
// imports is cached or read from export data (a package that imports one
// of the run's own would be invalid for the next run). When another run is
// checking the same package against the same imports, it waits for that
// check instead of making its own: the tests start several servers at once.
func (l *loader) checkCached(path string, lp *listPackage) *checked {
	target := l.targets[path]
	key := l.cache.packageKey(lp, l.overlay, func(name string) string { return filepath.Join(lp.Dir, name) })
	if key == "" {
		return l.checkSource(path)
	}
	imps, ok := l.importsOf(lp)
	if !ok || !l.storable(lp) {
		return l.checkSource(path)
	}
	for {
		e, f, own := l.ep.claim(path, key, target, imps)
		switch {
		case e != nil:
			l.mu.Lock()
			l.cached[path] = true
			l.reused[path] = true
			l.mu.Unlock()
			l.cache.mu.Lock()
			l.cache.reused++
			l.cache.mu.Unlock()
			return e.c
		case !own:
			<-f.done
			if f.stored {
				continue // the entry is there now
			}
			return l.checkSource(path)
		}
		c := l.checkSource(path)
		if c.pkg != nil {
			l.mu.Lock()
			l.cached[path] = true
			for _, name := range lp.GoFiles {
				l.kept[parseKey(filepath.Join(lp.Dir, name), target)] = true
			}
			l.mu.Unlock()
		}
		if l.ep.land(path, f, c) {
			l.cache.retire(l.ep)
		}
		return c
	}
}

// storable reports whether every import of lp is cached or read from
// export data; the imports are loaded.
func (l *loader) storable(lp *listPackage) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, imp := range lp.Imports {
		if m, ok := lp.ImportMap[imp]; ok {
			imp = m
		}
		if imp != "unsafe" && !l.exported[imp] && !l.cached[imp] {
			return false
		}
	}
	return true
}

// importsOf returns what each import of lp resolves to in this run, by the
// path the source names; ok is false when one cannot be imported.
func (l *loader) importsOf(lp *listPackage) (map[string]*types.Package, bool) {
	im := importer{l, lp}
	out := make(map[string]*types.Package, len(lp.Imports))
	for _, imp := range lp.Imports {
		p, err := im.Import(imp)
		if err != nil || p == nil {
			return nil, false
		}
		out[imp] = p
	}
	return out, true
}

func sameImports(a, b map[string]*types.Package) bool {
	if len(a) != len(b) {
		return false
	}
	for k, p := range a {
		if b[k] != p {
			return false
		}
	}
	return true
}

// release removes from the file set the files this run parsed that no
// cached package holds (the linted packages, their tests): the file set
// lives as long as the epoch, and an editor lints at every pause.
func (l *loader) release() {
	if l.cache == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for key, pf := range l.parsed {
		if l.kept[key] || pf.f == nil {
			continue
		}
		if tf := l.fset.File(pf.f.FileStart); tf != nil {
			l.fset.RemoveFile(tf)
		}
	}
}
