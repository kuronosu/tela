package fonts

import (
	_ "embed"
	"fmt"
)

// Font identifies one embedded font file.
type Font int

// The embedded fonts. The zero value is not a font.
const (
	// InterVariable is Inter 4.1, upright, variable (opsz, wght): the default sans.
	InterVariable Font = iota + 1
	// InterVariableItalic is Inter 4.1, italic, variable (opsz, wght).
	InterVariableItalic
	// JetBrainsMono is JetBrains Mono 2.304, upright, variable (wght): the default mono.
	JetBrainsMono
	// JetBrainsMonoItalic is JetBrains Mono 2.304, italic, variable (wght).
	JetBrainsMonoItalic
	// NotoSansArabic is Noto Sans Arabic, variable (wdth, wght).
	NotoSansArabic
	// NotoColorEmoji is Noto Color Emoji, COLRv1 build.
	NotoColorEmoji

	fontEnd
)

//go:embed inter/InterVariable.ttf
var interVariable []byte

//go:embed inter/InterVariable-Italic.ttf
var interVariableItalic []byte

//go:embed jetbrainsmono/JetBrainsMono-wght.ttf
var jetBrainsMono []byte

//go:embed jetbrainsmono/JetBrainsMono-Italic-wght.ttf
var jetBrainsMonoItalic []byte

//go:embed notosansarabic/NotoSansArabic-wdth-wght.ttf
var notoSansArabic []byte

//go:embed notocoloremoji/Noto-COLRv1.ttf
var notoColorEmoji []byte

// All returns every embedded font in a stable order.
func All() []Font {
	fs := make([]Font, 0, int(fontEnd)-1)
	for f := Font(1); f < fontEnd; f++ {
		fs = append(fs, f)
	}
	return fs
}

// Data returns the raw file bytes of f. The slice is shared: callers must not
// modify it. Data panics if f is not one of the constants of this package.
func Data(f Font) []byte {
	switch f {
	case InterVariable:
		return interVariable
	case InterVariableItalic:
		return interVariableItalic
	case JetBrainsMono:
		return jetBrainsMono
	case JetBrainsMonoItalic:
		return jetBrainsMonoItalic
	case NotoSansArabic:
		return notoSansArabic
	case NotoColorEmoji:
		return notoColorEmoji
	}
	panic(fmt.Sprintf("fonts: unknown font %d; use one of the constants of package fonts", int(f)))
}

// Name returns a short human-readable name of f, such as "Inter Variable".
func Name(f Font) string {
	switch f {
	case InterVariable:
		return "Inter Variable"
	case InterVariableItalic:
		return "Inter Variable Italic"
	case JetBrainsMono:
		return "JetBrains Mono"
	case JetBrainsMonoItalic:
		return "JetBrains Mono Italic"
	case NotoSansArabic:
		return "Noto Sans Arabic"
	case NotoColorEmoji:
		return "Noto Color Emoji"
	}
	return fmt.Sprintf("Font(%d)", int(f))
}
