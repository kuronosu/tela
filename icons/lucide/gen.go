package lucide

// Version is the Lucide release the icons were generated from. The module's
// tags follow it: the tag icons/lucide/v1.61.0 holds the icons of Lucide
// 1.61.0 (see README.md).
const Version = "1.61.0"

// icons_gen.go is generated from the SVG files of the lucide-static package,
// and deprecated.json (the names Lucide renamed, each with the icon that
// replaces it) from the same package. To regenerate them, unpack
// lucide-static somewhere (README.md says how) and run
//
//	LUCIDE_ICONS=/path/to/package/icons go generate ./...
//
// When Lucide releases a new version, change Version and the -source flag
// below together; a test checks that they agree.
//
//go:generate go run gen_deprecated.go $LUCIDE_ICONS
//go:generate go run github.com/kuronosu/tela/cmd/tela icons gen -pkg lucide -source "Lucide 1.61.0 (lucide-static, https://lucide.dev)" -license ISC -license-file LICENSE -deprecated deprecated.json $LUCIDE_ICONS
