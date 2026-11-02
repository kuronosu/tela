package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// appPackage writes the files of a package main to a temporary folder and
// returns it as go list would describe it, with the helper packages given
// as import path -> files (they belong to the main module).
func appPackage(t *testing.T, files map[string]string, helpers map[string]map[string]string) (listedPackage, map[string]listedPackage) {
	t.Helper()
	write := func(dir string, files map[string]string) []string {
		var names []string
		for name, src := range files {
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			names = append(names, name)
		}
		return names
	}
	root := t.TempDir()
	target := listedPackage{
		ImportPath: "example.com/app",
		Dir:        filepath.Join(root, "app"),
		Name:       "main",
		Module:     &listedModule{Path: "example.com", Main: true},
	}
	target.GoFiles = write(target.Dir, files)
	deps := map[string]listedPackage{
		// Not in the main module: an empty package stands for it.
		telaImportPath: {ImportPath: telaImportPath, Name: "tela", Module: &listedModule{Path: telaImportPath}},
	}
	for path, hf := range helpers {
		dir := filepath.Join(root, filepath.FromSlash(path))
		deps[path] = listedPackage{ImportPath: path, Dir: dir, Name: filepath.Base(path), GoFiles: write(dir, hf), Module: &listedModule{Path: "example.com", Main: true}}
	}
	return target, deps
}

func TestReadAppSource(t *testing.T) {
	for _, tc := range []struct {
		name    string
		files   map[string]string
		helpers map[string]map[string]string
		want    appSource // Found, ID, Title; errors are checked by wantErr
		idErr   string
		titErr  string
	}{
		{
			name: "var App literal",
			files: map[string]string{"main.go": `package main

import "github.com/kuronosu/tela"

var App = tela.App{Title: "Notes", AppID: "dev.example.notes", Width: 800}

func main() { tela.Main(App) }
`},
			want: appSource{Found: true, ID: "dev.example.notes", Title: "Notes"},
		},
		{
			name: "constants and concatenation",
			files: map[string]string{"main.go": `package main

import "github.com/kuronosu/tela"

const vendor = "dev.example"
const (
	id    = vendor + ".notes"
	title = "No" + "tes"
)

var App = tela.App{Title: title, AppID: id}

func main() { tela.Main(App) }
`},
			want: appSource{Found: true, ID: "dev.example.notes", Title: "Notes"},
		},
		{
			name: "wrapped in calls in main",
			files: map[string]string{"main.go": `package main

import (
	"github.com/kuronosu/tela"
	"github.com/kuronosu/tela/desktop"
)

var App = tela.App{Title: "Wrapped", AppID: "dev.example.wrapped"}

func with(a tela.App) tela.App { return a }

func main() {
	tela.Main(desktop.SingleInstance(with(desktop.PersistApp(App))))
}
`},
			want: appSource{Found: true, ID: "dev.example.wrapped", Title: "Wrapped"},
		},
		{
			name: "literal in tela.Main wins over var App",
			files: map[string]string{"b.go": `package main

import t "github.com/kuronosu/tela"

var App = t.App{AppID: "dev.example.other"}

func main() {
	t.Main(t.App{AppID: "dev.example.inline", Title: "Inline"})
}
`},
			want: appSource{Found: true, ID: "dev.example.inline", Title: "Inline"},
		},
		{
			name: "local variable and a pointer",
			files: map[string]string{"main.go": `package main

import "github.com/kuronosu/tela"

func main() {
	app := &tela.App{AppID: "dev.example.local"}
	tela.Run(*app)
}
`},
			want: appSource{Found: true, ID: "dev.example.local"},
		},
		{
			name: "dot import",
			files: map[string]string{"main.go": `package main

import . "github.com/kuronosu/tela"

var theApp = App{AppID: "dev.example.dot"}

func main() { Main(theApp) }
`},
			want: appSource{Found: true, ID: "dev.example.dot"},
		},
		{
			name: "constant from a package of the module",
			files: map[string]string{"main.go": `package main

import (
	"example.com/meta"
	"github.com/kuronosu/tela"
)

var App = tela.App{AppID: meta.ID, Title: meta.Name + " app"}

func main() { tela.Main(App) }
`},
			helpers: map[string]map[string]string{"example.com/meta": {"meta.go": `package meta

const Name = "Meta"
const ID = "dev.example." + "meta"
`}},
			want: appSource{Found: true, ID: "dev.example.meta", Title: "Meta app"},
		},
		{
			name: "missing ID",
			files: map[string]string{"main.go": `package main

import "github.com/kuronosu/tela"

func main() { tela.Main(tela.App{Title: "Demo"}) }
`},
			want: appSource{Found: true, Title: "Demo"},
		},
		{
			name: "ID computed at run time",
			files: map[string]string{"main.go": `package main

import (
	"os"
	"github.com/kuronosu/tela"
)

var App = tela.App{AppID: os.Getenv("ID"), Title: "T"}

func main() { tela.Main(App) }
`},
			want:  appSource{Found: true, Title: "T"},
			idErr: "AppID of the tela.App is not a constant string",
		},
		{
			name: "no App at all",
			files: map[string]string{"main.go": `package main

type App struct{ AppID string }

var other = App{AppID: "not.tela"}

func main() {}
`},
			want: appSource{},
		},
		{
			name: "another App type is ignored",
			files: map[string]string{"main.go": `package main

import "example.com/other"

var App = other.App{AppID: "not.tela"}

func main() {}
`},
			helpers: map[string]map[string]string{"example.com/other": {"o.go": "package other\n\ntype App struct{ AppID string }\n"}},
			want:    appSource{},
		},
		{
			name: "a cycle of variables does not hang",
			files: map[string]string{"main.go": `package main

import "github.com/kuronosu/tela"

var a = b
var b = a
var _ tela.App

func main() { tela.Main(a) }
`},
			want: appSource{},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			target, deps := appPackage(t, tc.files, tc.helpers)
			got, err := readAppSource(target, deps)
			if err != nil {
				t.Fatal(err)
			}
			if got.Found != tc.want.Found || got.ID != tc.want.ID || got.Title != tc.want.Title {
				t.Errorf("got Found=%v ID=%q Title=%q, want Found=%v ID=%q Title=%q", got.Found, got.ID, got.Title, tc.want.Found, tc.want.ID, tc.want.Title)
			}
			checkErr := func(what string, err error, want string) {
				switch {
				case want == "" && err != nil:
					t.Errorf("%s: unexpected error %v", what, err)
				case want != "" && (err == nil || !strings.Contains(err.Error(), want)):
					t.Errorf("%s: error %v, want it to contain %q", what, err, want)
				}
			}
			checkErr("AppID", got.IDErr, tc.idErr)
			checkErr("Title", got.TitleErr, tc.titErr)
		})
	}
}

func TestReadAppSourceSyntaxError(t *testing.T) {
	target, deps := appPackage(t, map[string]string{"main.go": "package main\n\nfunc main( {\n"}, nil)
	if _, err := readAppSource(target, deps); err == nil || !strings.Contains(err.Error(), "main.go") {
		t.Errorf("syntax error: %v", err)
	}
}
