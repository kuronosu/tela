// Package hyphen holds the hyphenation dictionaries of Tela's opt-in
// language packages (hyphen/en, hyphen/es, hyphen/fr; ADR-270): TeX
// hyphenation patterns vendored in src/ (with their licenses; sources in
// THIRD_PARTY.md), compiled by gen into the compact form that the leaf
// packages en, es and fr embed. Only the packages that choose to embed a
// dictionary import those, so a binary that imports none carries no
// patterns. Parsing and hyphenation are done by internal/text.
package hyphen

//go:generate go run ./gen
