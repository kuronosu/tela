package main

import (
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// TestFilmGeneratedSource checks that the generated files parse with and
// without the film feature, and that a tela module that predates
// App.Film gets a file that compiles against it (no use of FilmFrame).
func TestFilmGeneratedSource(t *testing.T) {
	for _, film := range []bool{false, true} {
		feats := genFeatures{Film: film}
		for name, src := range map[string][]byte{
			"mcp":      generateMCPSource("widgets", contractPreviews, true, feats),
			"snapshot": generateSource("widgets", contractPreview, false, feats),
		} {
			if _, err := parser.ParseFile(token.NewFileSet(), name+".go", src, parser.AllErrors); err != nil {
				t.Fatalf("%s (film %v): %v\n%s", name, film, err, src)
			}
			s := string(src)
			if has := strings.Contains(s, "app.Film("); has != film {
				t.Errorf("%s: uses App.Film = %v with the feature %v", name, has, film)
			}
			if !film && strings.Contains(s, "telatest.FilmFrame") {
				t.Errorf("%s: names telatest.FilmFrame, which an older tela lacks", name)
			}
		}
	}
}
