package text

import (
	"github.com/kuronosu/tela/internal/assets/hyphen/en"
	"github.com/kuronosu/tela/internal/assets/hyphen/es"
	"github.com/kuronosu/tela/internal/assets/hyphen/fr"
)

// The dictionaries of the hyphen packages, for tests.
func testHyphenUS() *Hyphenator { return NewHyphenator("en-US", en.US, "") }
func testHyphenGB() *Hyphenator { return NewHyphenator("en-GB", en.GB, "") }
func testHyphenES() *Hyphenator { return NewHyphenator("es", es.Spanish, "") }
func testHyphenFR() *Hyphenator { return NewHyphenator("fr", fr.French, "") }
