package detect

import (
	"strconv"
	"strings"

	"github.com/kuronosu/tela/looks/aqua"
)

// macOSTahoe is the first version of macOS (its major number) that draws
// Liquid Glass.
const macOSTahoe = 26

// macOSDesign is the design of Aqua that a macOS version shows: Tahoe from
// macOS 26, Sequoia before it (macOS 11 to 15, and 10.16, which is how a
// program linked against an old SDK sees Big Sur). A version that cannot be
// read ("" on a system that is not macOS, or something unexpected) is the
// current design, Tahoe.
func macOSDesign(version string) aqua.Design {
	major, _, _ := strings.Cut(strings.TrimSpace(version), ".")
	n, err := strconv.Atoi(major)
	if err != nil || n >= macOSTahoe {
		return aqua.Tahoe
	}
	return aqua.Sequoia
}
