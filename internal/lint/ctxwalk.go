package lint

import (
	"bufio"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// Listing the context. Options.Context is a whole module for tela lsp, and
// go list -deps of a module is most of a lint after the first one (0.3 s
// for Tela's repository, as long as listing the export data of everything
// it reaches). Only the packages that mention ClassAliases or SchemeColor
// can be context (mentionsAliases), with what they import: when every
// pattern is a directory tree ("dir/..."), the run walks the trees itself,
// as the go command walks them for "...", finds the directories with a Go
// file that mentions one of them, and lists only those, with -deps. The
// packages it leaves out were dependencies that nothing imports; the
// context is the same. Any other pattern is listed as given.

// contextDirs returns the directories to list for Options.Context, or ok
// false when a pattern is not a directory tree.
func (r *run) contextDirs() (dirs []string, ok bool) {
	for _, pat := range r.opts.Context {
		root, found := strings.CutSuffix(filepath.ToSlash(pat), "/...")
		if !found || strings.Contains(root, "...") || !(filepath.IsAbs(root) || root == "." || root == ".." || strings.HasPrefix(root, "./") || strings.HasPrefix(root, "../")) {
			return nil, false
		}
		root = filepath.FromSlash(root)
		if !filepath.IsAbs(root) {
			root = filepath.Join(r.dir, root)
		}
		fi, err := os.Stat(root)
		if err != nil || !fi.IsDir() {
			return nil, false // the go command reports it; let it
		}
		mod, modDir := moduleAt(root)
		dirs = append(dirs, r.walkContext(root, mod, modDir)...)
	}
	return dirs, true
}

// walkContext returns the directories under root that "root/..." matches
// and that have a Go file mentioning ClassAliases or SchemeColor. It walks
// as the go command does: directories whose name starts with "." or "_",
// testdata and vendor are left out, and so are nested modules (a go.mod)
// and symbolic links. mod is the import path of root's module, whose
// folder is modDir, "" when it is not known; Tela's own packages are left
// out (telaOwn).
func (r *run) walkContext(root, mod, modDir string) []string {
	var out []string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if d != nil && d.IsDir() && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.IsDir() {
			return nil
		}
		if path != root {
			name := d.Name()
			if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor" {
				return filepath.SkipDir
			}
			if _, err := os.Stat(filepath.Join(path, "go.mod")); err == nil {
				return filepath.SkipDir
			}
		}
		if mod != "" {
			imp := mod
			if rel, err := filepath.Rel(modDir, path); err == nil && rel != "." {
				imp += "/" + filepath.ToSlash(rel)
			}
			if telaOwn(&listPackage{ImportPath: imp, Module: &struct {
				Path      string
				GoVersion string
			}{Path: mod}}) {
				return nil // its subdirectories may be examples
			}
		}
		if r.dirMentionsAliases(path) {
			out = append(out, path)
		}
		return nil
	})
	return out
}

// dirMentionsAliases reports whether a Go file of dir that can be in its
// package (not a test, not ignored by name) mentions ClassAliases or
// SchemeColor. Build constraints are left to the go command: a package it
// lists is checked again with mentionsAliases.
func (r *run) dirMentionsAliases(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") ||
			strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		if mentionsFile(filepath.Join(dir, name), r.opts.Cache) {
			return true
		}
	}
	return false
}

// moduleAt returns the path of the module that dir is in, from the go.mod
// at or above it, and the folder of that go.mod; "" when there is none.
func moduleAt(dir string) (path, modDir string) {
	for {
		f, err := os.Open(filepath.Join(dir, "go.mod"))
		if err == nil {
			defer f.Close()
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				line := strings.TrimSpace(sc.Text())
				rest, ok := strings.CutPrefix(line, "module")
				if !ok || rest == "" || (rest[0] != ' ' && rest[0] != '\t') {
					continue
				}
				rest, _, _ = strings.Cut(strings.TrimSpace(rest), "//")
				rest = strings.TrimSpace(rest)
				if uq, err := strconv.Unquote(rest); err == nil {
					rest = uq
				}
				return rest, dir
			}
			return "", ""
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ""
		}
		dir = parent
	}
}
