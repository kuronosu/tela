package text

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/go-text/typesetting/font"
	ot "github.com/go-text/typesetting/font/opentype"
	"github.com/go-text/typesetting/font/opentype/tables"
	"github.com/go-text/typesetting/fontscan"
)

// defaultFontDirectories lists the operating system's font directories; a
// variable so that tests can point the scan at a temporary directory.
var defaultFontDirectories = fontscan.DefaultFontDirectories

// systemFootprints lists the fonts of the operating system's directories.
//
// It uses fontscan's index, which is stored in the cache directory and only
// rescans the files whose modification time changed. fontscan builds the
// index once per process and fails for the whole process when it cannot
// write it, so the directory is checked first; if it is not usable, or
// NoCache is set, the directories are scanned here without a cache.
func (s *systemFonts) systemFootprints() ([]fontscan.Footprint, error) {
	if !s.opts.NoCache {
		dir, err := cacheDirectory(s.opts.CacheDir)
		if err == nil {
			return fontscan.SystemFonts(s.logger, dir)
		}
		s.logger.Printf("font index cache unusable, scanning without it: %v", err)
	}
	dirs, err := defaultFontDirectories(s.logger)
	if err != nil {
		return nil, err
	}
	var fps []fontscan.Footprint
	for _, dir := range dirs {
		fps = append(fps, scanDirectory(dir, s.logger)...)
	}
	return fps, nil
}

// cacheDirectory returns dir (default: os.UserCacheDir()/tela/fonts) after
// making sure it exists and can be written to.
func cacheDirectory(dir string) (string, error) {
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "tela", "fonts")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	probe, err := os.CreateTemp(dir, ".probe-*")
	if err != nil {
		return "", err
	}
	probe.Close()
	os.Remove(probe.Name())
	return dir, nil
}

// scanDirectory returns the footprints of the font files under dir
// (TrueType, OpenType and collections), skipping what cannot be read. It is
// fontscan's scan without its cache, which fontscan only offers for the
// operating system's directories.
func scanDirectory(dir string, logger SystemFontLogger) []fontscan.Footprint {
	var fps []fontscan.Footprint
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			logger.Printf("error walking font directory %q: %v", path, err)
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".ttf", ".otf", ".ttc", ".otc":
		default:
			return nil
		}
		got, err := fileFootprints(path)
		if err != nil {
			logger.Printf("skipping font file %q: %v", path, err)
			return nil
		}
		fps = append(fps, got...)
		return nil
	})
	if err != nil {
		logger.Printf("scanning font directory %q: %v", dir, err)
	}
	return fps
}

// fileFootprints returns the footprint of every font in a file.
func fileFootprints(path string) ([]fontscan.Footprint, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	lds, err := ot.NewLoaders(f)
	if err != nil {
		return nil, err
	}
	var out []fontscan.Footprint
	for i, ld := range lds {
		fp, err := footprintOf(ld)
		if err != nil {
			continue // not a usable font of the file
		}
		fp.Location.File = path
		fp.Location.Index = uint16(i)
		out = append(out, fp)
	}
	return out, nil
}

// footprintOf summarizes a font like fontscan does: family, aspect and the
// runes its cmap maps to a glyph. Scripts and languages are not needed here.
func footprintOf(ld *ot.Loader) (fontscan.Footprint, error) {
	var fp fontscan.Footprint
	page := tables.FPNone
	if raw, err := ld.RawTable(ot.MustNewTag("OS/2")); err == nil {
		if os2, _, err := tables.ParseOs2(raw); err == nil {
			page = os2.FontPage()
		}
	}
	raw, err := ld.RawTable(ot.MustNewTag("cmap"))
	if err != nil {
		return fp, err
	}
	tb, _, err := tables.ParseCmap(raw)
	if err != nil {
		return fp, err
	}
	cmap, _, err := font.ProcessCmap(tb, page)
	if err != nil {
		return fp, err
	}
	if ranger, ok := cmap.(font.CmapRuneRanger); ok {
		addRuneRanges(&fp.Runes, ranger.RuneRanges(nil))
	} else {
		for it := cmap.Iter(); it.Next(); {
			if r, gid := it.Char(); gid != 0 {
				fp.Runes.Add(r)
			}
		}
	}
	desc, _ := font.Describe(ld, nil)
	if desc.Family == "" {
		return fp, errors.New("font without a family name")
	}
	fp.Family = font.NormalizeFamily(desc.Family)
	fp.Aspect = desc.Aspect
	return fp, nil
}

// addRuneRanges adds the runes of ranges to rs, clamped to the Unicode code
// space: a malformed cmap can map ranges up to 0xFFFFFFFF, which would never
// end a loop over int32 runes and would mark pages outside Unicode.
func addRuneRanges(rs *fontscan.RuneSet, ranges [][2]rune) {
	for _, rg := range ranges {
		lo, hi := max(rg[0], 0), min(rg[1], unicode.MaxRune)
		for r := lo; r <= hi; r++ {
			rs.Add(r)
		}
	}
}
