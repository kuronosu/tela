package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
)

// watchSet decides what tela dev watches and which changes rebuild: the Go
// sources (non-test .go and .s files), go.mod, go.sum and go.work of the
// modules the app builds from source; the files their packages embed; and
// anything under the asset directories the app reported. It is read by the
// watcher's goroutines and updated by the supervisor, under a lock.
type watchSet struct {
	tmp string // the build directory, never watched

	mu      sync.RWMutex
	modules []string
	embeds  map[string]bool
	pkgDirs map[string]bool
	assets  []string
}

func newWatchSet(tmp string) *watchSet {
	return &watchSet{tmp: filepath.Clean(tmp), embeds: map[string]bool{}, pkgDirs: map[string]bool{}}
}

// setDeps replaces the modules and embedded files (after a build).
func (s *watchSet) setDeps(d depSet) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.modules = cleanAll(d.modules)
	s.embeds = map[string]bool{}
	for _, f := range d.embeds {
		s.embeds[filepath.Clean(f)] = true
	}
	s.pkgDirs = map[string]bool{}
	for _, dir := range d.pkgDirs {
		s.pkgDirs[filepath.Clean(dir)] = true
	}
}

// setAssets replaces the asset directories.
func (s *watchSet) setAssets(dirs []string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.assets = cleanAll(dirs)
}

// roots returns the directories to watch.
func (s *watchSet) roots() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r := slices.Clone(s.modules)
	for _, a := range s.assets {
		if !slices.Contains(r, a) {
			r = append(r, a)
		}
	}
	return r
}

func cleanAll(paths []string) []string {
	var out []string
	for _, p := range paths {
		if p == "" {
			continue
		}
		if abs, err := filepath.Abs(p); err == nil {
			p = abs
		}
		p = filepath.Clean(p)
		if !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return out
}

// under returns the first of dirs that contains path, or "".
func under(path string, dirs []string) string {
	for _, d := range dirs {
		if path == d || strings.HasPrefix(path, d+string(filepath.Separator)) {
			return d
		}
	}
	return ""
}

// skipDir reports whether a directory inside a watched root is not
// watched: hidden directories (.git, .idea, .vscode), node_modules, the
// build directory and nested modules (a directory with its own go.mod that
// is not one of the watched modules), unless it holds an embedded file.
func (s *watchSet) skipDir(path string) bool {
	path = filepath.Clean(path)
	s.mu.RLock()
	defer s.mu.RUnlock()
	if path == s.tmp {
		return true
	}
	base := filepath.Base(path)
	skip := strings.HasPrefix(base, ".") || base == "node_modules"
	if !skip && under(path, s.modules) != "" && !slices.Contains(s.modules, path) {
		if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
			skip = true
		}
	}
	if !skip {
		return false
	}
	// An embed pattern with the all: prefix may reach into a hidden
	// directory.
	for f := range s.embeds {
		if strings.HasPrefix(f, path+string(filepath.Separator)) {
			return false
		}
	}
	return true
}

// relevant reports whether a change of path should rebuild the app. dir
// is true for a directory that left the tree.
func (s *watchSet) relevant(path string, dir bool) bool {
	path = filepath.Clean(path)
	base := filepath.Base(path)
	if editorTemp(base) {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.embeds[path] || under(path, s.assets) != "" {
		return true
	}
	mod := under(path, s.modules)
	if mod == "" {
		return false
	}
	if dir {
		return true // it may have held packages
	}
	switch base {
	case "go.mod", "go.sum", "go.work", "go.work.sum":
		return true
	}
	ext := filepath.Ext(base)
	if (ext != ".go" && ext != ".s") || strings.HasSuffix(base, "_test.go") {
		return false
	}
	// Go files in testdata are not part of any build, unless the app is
	// a package there (built directly, as tela's own test apps are).
	rel, err := filepath.Rel(mod, path)
	if err != nil {
		return false
	}
	return !slices.Contains(strings.Split(filepath.ToSlash(rel), "/"), "testdata") || s.pkgDirs[filepath.Dir(path)]
}

// editorTemp reports whether a file name is an editor's temporary or
// backup file: vim (.swp, .swo, .swx, ~ backups, the 4913 probe), Emacs
// (#auto-save#, .#lock), GoLand (___jb_tmp___, ___jb_old___), Kate and
// others (.tmp), Chromium-based editors (.crswap).
func editorTemp(base string) bool {
	switch {
	case base == "4913",
		strings.HasSuffix(base, "~"),
		strings.HasPrefix(base, ".#"),
		strings.HasPrefix(base, "#") && strings.HasSuffix(base, "#"),
		strings.HasSuffix(base, ".swp"), strings.HasSuffix(base, ".swo"),
		strings.HasSuffix(base, ".swx"), strings.HasSuffix(base, ".swpx"),
		strings.HasSuffix(base, ".tmp"), strings.HasSuffix(base, ".crswap"),
		strings.Contains(base, "___jb_tmp___"), strings.Contains(base, "___jb_old___"):
		return true
	}
	return false
}
