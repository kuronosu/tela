package assoc

import (
	"bufio"
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
)

// The shared-mime-info side of file types: which MIME type claims an
// extension, and the app's own MIME package.

const mimeNS = "http://www.freedesktop.org/standards/shared-mime-info"

// replaceWeight is the glob weight of a MIME package entry written with
// RegisterOptions.Replace over an extension another type claims: above
// the default 50 of the other type's glob, so the new type wins the match
// (shared-mime-info: the highest weight wins among literal patterns).
const replaceWeight = 80

// mimePackage is the app's own package, $XDG_DATA_HOME/mime/packages/<ID>.xml:
// the MIME types it defines, in order, with their comments and globs.
type mimePackage struct {
	types []mimeDef
}

type mimeDef struct {
	typ     string
	comment string
	globs   []mimeGlob
}

type mimeGlob struct {
	pattern string
	weight  int // 0: the default (50)
}

// xmlPackage is what parsing a MIME package reads.
type xmlPackage struct {
	Types []struct {
		Type     string `xml:"type,attr"`
		Comments []struct {
			Lang string `xml:"http://www.w3.org/XML/1998/namespace lang,attr"`
			Text string `xml:",chardata"`
		} `xml:"comment"`
		Globs []struct {
			Pattern string `xml:"pattern,attr"`
			Weight  string `xml:"weight,attr"`
		} `xml:"glob"`
	} `xml:"mime-type"`
}

func parseMIMEPackage(data []byte) (*mimePackage, error) {
	var x xmlPackage
	if err := xml.Unmarshal(data, &x); err != nil {
		return nil, err
	}
	p := &mimePackage{}
	for _, t := range x.Types {
		d := mimeDef{typ: t.Type}
		for _, c := range t.Comments {
			if c.Lang == "" {
				d.comment = strings.TrimSpace(c.Text)
			}
		}
		for _, g := range t.Globs {
			w, _ := strconv.Atoi(g.Weight)
			d.globs = append(d.globs, mimeGlob{pattern: g.Pattern, weight: w})
		}
		p.types = append(p.types, d)
	}
	return p, nil
}

// bytes renders the package; nil when it defines nothing (the file is
// removed then).
func (p *mimePackage) bytes(appID string) []byte {
	if len(p.types) == 0 {
		return nil
	}
	esc := func(s string) string {
		var b bytes.Buffer
		_ = xml.EscapeText(&b, []byte(s))
		return b.String()
	}
	var b strings.Builder
	b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n")
	b.WriteString("<!-- Written by Tela for " + esc(appID) + " (platform.Associations); removed by Unregister. -->\n")
	b.WriteString("<mime-info xmlns=\"" + mimeNS + "\">\n")
	for _, d := range p.types {
		b.WriteString("  <mime-type type=\"" + esc(d.typ) + "\">\n")
		b.WriteString("    <comment>" + esc(d.comment) + "</comment>\n")
		for _, g := range d.globs {
			b.WriteString("    <glob pattern=\"" + esc(g.pattern) + "\"")
			if g.weight != 0 {
				b.WriteString(" weight=\"" + strconv.Itoa(g.weight) + "\"")
			}
			b.WriteString("/>\n")
		}
		b.WriteString("  </mime-type>\n")
	}
	b.WriteString("</mime-info>\n")
	return []byte(b.String())
}

// extPattern is the glob of an extension: "*.note" for ".note", lower-cased.
func extPattern(ext string) string { return "*" + strings.ToLower(ext) }

// globType returns the MIME type the package maps ext to, or "".
func (p *mimePackage) globType(ext string) string {
	pat := extPattern(ext)
	for _, d := range p.types {
		for _, g := range d.globs {
			if strings.EqualFold(g.pattern, pat) {
				return d.typ
			}
		}
	}
	return ""
}

// setGlob maps ext to typ (with comment and weight), removing a mapping of
// ext to another type of the package.
func (p *mimePackage) setGlob(ext, typ, comment string, weight int) {
	p.removeGlob(ext)
	pat := extPattern(ext)
	for i := range p.types {
		if strings.EqualFold(p.types[i].typ, typ) {
			p.types[i].comment = comment
			p.types[i].globs = append(p.types[i].globs, mimeGlob{pattern: pat, weight: weight})
			return
		}
	}
	p.types = append(p.types, mimeDef{typ: typ, comment: comment, globs: []mimeGlob{{pattern: pat, weight: weight}}})
}

// removeGlob removes the mapping of ext, and a type left without globs.
func (p *mimePackage) removeGlob(ext string) bool {
	pat := extPattern(ext)
	removed := false
	for i := range p.types {
		n := len(p.types[i].globs)
		p.types[i].globs = slices.DeleteFunc(p.types[i].globs, func(g mimeGlob) bool { return strings.EqualFold(g.pattern, pat) })
		removed = removed || len(p.types[i].globs) != n
	}
	p.types = slices.DeleteFunc(p.types, func(d mimeDef) bool { return len(d.globs) == 0 })
	return removed
}

// extClaims returns the MIME types that other software maps ext to, with
// their highest glob weight: the compiled databases of the system
// (<DataDirs>/mime/globs2) and the user's MIME packages other than the
// app's own (<DataHome>/mime/packages/*.xml, the source of the user's
// database, read directly so that a package whose database was not
// rebuilt counts too). Only the literal pattern "*<ext>" is considered,
// without regard to case.
func (l *linuxAssoc) extClaims(ext, ownPackage string) map[string]int {
	pat := extPattern(ext)
	claims := map[string]int{}
	claim := func(typ string, w int) {
		typ = strings.ToLower(typ)
		if old, ok := claims[typ]; !ok || w > old {
			claims[typ] = w
		}
	}
	for _, dir := range l.dirs.DataDirs {
		f, err := os.Open(filepath.Join(dir, "mime", "globs2"))
		if err != nil {
			continue
		}
		sc := bufio.NewScanner(f)
		for sc.Scan() {
			line := sc.Text()
			if line == "" || line[0] == '#' {
				continue
			}
			// weight:type:pattern[:flags[:…]]
			parts := strings.SplitN(line, ":", 4)
			if len(parts) < 3 || !strings.EqualFold(parts[2], pat) {
				continue
			}
			w, err := strconv.Atoi(parts[0])
			if err != nil {
				continue
			}
			claim(parts[1], w)
		}
		f.Close()
	}
	pkgs, _ := filepath.Glob(filepath.Join(l.dirs.DataHome, "mime", "packages", "*.xml"))
	for _, path := range pkgs {
		if path == ownPackage {
			continue
		}
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		p, err := parseMIMEPackage(data)
		if err != nil {
			continue
		}
		for _, d := range p.types {
			for _, g := range d.globs {
				if strings.EqualFold(g.pattern, pat) {
					w := g.weight
					if w == 0 {
						w = 50
					}
					claim(d.typ, w)
				}
			}
		}
	}
	return claims
}

// topClaim returns the type with the highest weight among claims ("" for
// none; ties are broken by name, for determinism).
func topClaim(claims map[string]int) string {
	best, bw := "", -1
	for t, w := range claims {
		if w > bw || w == bw && t < best {
			best, bw = t, w
		}
	}
	return best
}
