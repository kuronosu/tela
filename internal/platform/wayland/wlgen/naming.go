package main

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// camel converts a protocol name (wl_shm_pool) to an exported Go name
// (WlShmPool). Digits are kept as they are, so "wp_viewport_v1" becomes
// "WpViewportV1" and the entry "90" becomes "90".
func camel(s string) string {
	var b strings.Builder
	for _, part := range strings.Split(s, "_") {
		if part == "" {
			continue
		}
		r, n := utf8.DecodeRuneInString(part)
		b.WriteRune(unicode.ToUpper(r))
		b.WriteString(part[n:])
	}
	return b.String()
}

// lowerCamel converts a protocol name to an unexported Go name (xDest).
func lowerCamel(s string) string {
	c := camel(s)
	if c == "" {
		return c
	}
	r, n := utf8.DecodeRuneInString(c)
	return string(unicode.ToLower(r)) + c[n:]
}

var goKeywords = map[string]bool{
	"break": true, "default": true, "func": true, "interface": true, "select": true,
	"case": true, "defer": true, "go": true, "map": true, "struct": true,
	"chan": true, "else": true, "goto": true, "package": true, "switch": true,
	"const": true, "fallthrough": true, "if": true, "range": true, "type": true,
	"continue": true, "for": true, "import": true, "return": true, "var": true,
}

// reservedLocals are identifiers the generated method bodies use; a protocol
// argument with one of these names gets a trailing underscore.
var reservedLocals = map[string]bool{
	"self": true, "msg": true, "wire": true, "fmt": true, "proxy_": true,
	"obj": true, "version": true, "ev": true, "r": true, "h": true,
	"err": true, "opcode": true, "nil": true, "true": true, "false": true,
	"string": true, "int32": true, "uint32": true, "float64": true, "int": true,
}

// paramName turns a protocol argument name into a Go parameter name.
func paramName(s string) string {
	n := lowerCamel(s)
	if goKeywords[n] || reservedLocals[n] {
		n += "_"
	}
	return n
}

// reservedMethods are the methods of wire.Proxy and of the Object interface;
// a request or event setter must not shadow them.
var reservedMethods = map[string]bool{
	"Base": true, "ID": true, "Version": true, "Conn": true, "Destroyed": true,
	"String": true, "Begin": true, "MarkDestroyed": true, "Descriptor": true, "Dispatch": true,
}
