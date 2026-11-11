// Command gen compiles the vendored TeX hyphenation patterns of
// internal/assets/hyphen/src into the compact form the hyphenation
// packages embed (text.EncodeHyphenPatterns: the patterns and exceptions
// without comments, sorted, compressed). Run it with go generate in
// internal/assets/hyphen; the outputs are committed.
package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kuronosu/tela/internal/text"
)

// dict is one dictionary: its source files in src/<name>/ and its output.
type dict struct {
	name string // hyph-<name>.pat.txt and the optional hyph-<name>.hyp.txt
	out  string
}

var dicts = []dict{
	{"en-us", "en/en-us.hyph"},
	{"en-gb", "en/en-gb.hyph"},
	{"es", "es/es.hyph"},
	{"fr", "fr/fr.hyph"},
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "gen:", err)
		os.Exit(1)
	}
}

func run() error {
	for _, d := range dicts {
		base := filepath.Join("src", d.name, "hyph-"+d.name)
		pat, err := os.ReadFile(base + ".pat.txt")
		if err != nil {
			return err
		}
		hyp, err := os.ReadFile(base + ".hyp.txt")
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		enc, err := text.EncodeHyphenPatterns(string(pat), string(hyp))
		if err != nil {
			return fmt.Errorf("%s: %v", d.name, err)
		}
		if err := os.WriteFile(d.out, []byte(enc), 0o644); err != nil {
			return err
		}
		fmt.Printf("%s: %d bytes\n", d.out, len(enc))
	}
	return nil
}
