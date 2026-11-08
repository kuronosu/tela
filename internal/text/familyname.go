package text

import (
	"bytes"
	"errors"
	"fmt"
	"strings"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
)

// FamilyName returns the family name a font file declares (its typographic
// family, else its family), read from the name table without parsing the
// font. For a file of several fonts (a collection) it is the first one's.
func FamilyName(data []byte) (string, error) {
	lds, err := ot.NewLoaders(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("text: reading font: %w", err)
	}
	if len(lds) == 0 {
		return "", errors.New("text: the file has no font")
	}
	desc, _ := font.Describe(lds[0], nil)
	name := strings.TrimSpace(desc.Family)
	if name == "" {
		return "", errors.New("text: the font declares no family name")
	}
	return name, nil
}
