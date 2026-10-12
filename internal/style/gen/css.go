package main

import (
	"fmt"
	"strings"
)

// themeVar is one custom property declared in an @theme block, in source
// order.
type themeVar struct {
	Name  string // "--color-red-50"
	Value string // "oklch(97.1% 0.013 17.38)", whitespace collapsed
	// Deprecated is set for variables of an "@theme ... reference" block,
	// which Tailwind keeps only for its v3 aliases ("rounded", "shadow").
	Deprecated bool
}

// parseThemeCSS extracts the custom properties declared directly inside
// @theme blocks. Comments and nested at-rules (@keyframes) are skipped.
// Anything outside an @theme block is rejected so a Tailwind update that
// adds new kinds of rules is noticed instead of silently dropped.
func parseThemeCSS(src string) ([]themeVar, error) {
	s := stripComments(src)
	var vars []themeVar
	for {
		s = strings.TrimSpace(s)
		if s == "" {
			return vars, nil
		}
		open := strings.IndexByte(s, '{')
		if open < 0 {
			return nil, fmt.Errorf("theme.css: unexpected text outside @theme blocks: %q", clip(s))
		}
		prelude := strings.Fields(s[:open])
		if len(prelude) == 0 || prelude[0] != "@theme" {
			return nil, fmt.Errorf("theme.css: unsupported top-level rule %q; the generator reads only @theme blocks", clip(s[:open]))
		}
		end, err := matchBrace(s, open)
		if err != nil {
			return nil, err
		}
		deprecated := false
		for _, w := range prelude[1:] {
			if w == "reference" {
				deprecated = true
			}
		}
		body := s[open+1 : end]
		bv, err := parseThemeBody(body, deprecated)
		if err != nil {
			return nil, err
		}
		vars = append(vars, bv...)
		s = s[end+1:]
	}
}

func parseThemeBody(body string, deprecated bool) ([]themeVar, error) {
	var vars []themeVar
	for {
		body = strings.TrimSpace(body)
		if body == "" {
			return vars, nil
		}
		if body[0] == '@' {
			// A nested at-rule such as @keyframes: skip its block.
			open := strings.IndexByte(body, '{')
			if open < 0 {
				return nil, fmt.Errorf("theme.css: at-rule without a block: %q", clip(body))
			}
			end, err := matchBrace(body, open)
			if err != nil {
				return nil, err
			}
			body = body[end+1:]
			continue
		}
		semi := strings.IndexByte(body, ';')
		if semi < 0 {
			return nil, fmt.Errorf("theme.css: declaration without ';': %q", clip(body))
		}
		decl := body[:semi]
		body = body[semi+1:]
		colon := strings.IndexByte(decl, ':')
		if colon < 0 {
			return nil, fmt.Errorf("theme.css: malformed declaration %q", clip(decl))
		}
		name := strings.TrimSpace(decl[:colon])
		if !strings.HasPrefix(name, "--") {
			return nil, fmt.Errorf("theme.css: %q is not a custom property", name)
		}
		vars = append(vars, themeVar{
			Name:       name,
			Value:      strings.Join(strings.Fields(decl[colon+1:]), " "),
			Deprecated: deprecated,
		})
	}
}

// matchBrace returns the index of the '}' that closes the '{' at s[open].
func matchBrace(s string, open int) (int, error) {
	depth := 0
	for i := open; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i, nil
			}
		}
	}
	return 0, fmt.Errorf("theme.css: unbalanced '{' in %q", clip(s[open:]))
}

func stripComments(s string) string {
	var b strings.Builder
	for {
		i := strings.Index(s, "/*")
		if i < 0 {
			b.WriteString(s)
			return b.String()
		}
		b.WriteString(s[:i])
		j := strings.Index(s[i+2:], "*/")
		if j < 0 {
			return b.String()
		}
		s = s[i+2+j+2:]
	}
}

func clip(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 60 {
		return s[:60] + "..."
	}
	return s
}

// splitTop splits s on sep, ignoring separators inside parentheses or
// quotes, and trims each piece.
func splitTop(s string, sep byte) []string {
	var parts []string
	depth := 0
	var quote byte
	start := 0
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case quote != 0:
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case c == '(':
			depth++
		case c == ')':
			depth--
		case c == sep && depth == 0:
			parts = append(parts, strings.TrimSpace(s[start:i]))
			start = i + 1
		}
	}
	return append(parts, strings.TrimSpace(s[start:]))
}

// fieldsTop splits s on whitespace, keeping parenthesized groups whole.
func fieldsTop(s string) []string {
	var parts []string
	depth := 0
	start := -1
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '(':
			depth++
		case c == ')':
			depth--
		}
		space := c == ' ' || c == '\t' || c == '\n'
		if space && depth == 0 {
			if start >= 0 {
				parts = append(parts, s[start:i])
				start = -1
			}
		} else if start < 0 {
			start = i
		}
	}
	if start >= 0 {
		parts = append(parts, s[start:])
	}
	return parts
}
