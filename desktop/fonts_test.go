package desktop_test

// The tests draw text with the deterministic fonts (ADR-008, ADR-244):
// importing internal/stdfonts registers them as the default of trees
// without fonts, instead of the system's (which are off under go test).
import _ "github.com/kuronosu/tela/internal/stdfonts"
