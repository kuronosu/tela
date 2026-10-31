package style

import (
	"slices"
	"testing"
)

func TestLexCSS(t *testing.T) {
	num := func(v float32, unit string) cssToken { return cssToken{kind: 'n', num: v, unit: unit} }
	id := func(s string) cssToken { return cssToken{kind: 'i', text: s} }
	opTok := func(c byte) cssToken { return cssToken{kind: c} }

	tests := []struct {
		name string
		in   string
		want []cssToken
		err  string
	}{
		{name: "empty", in: ""},
		{name: "spaces and tabs", in: " \t1 \t", want: []cssToken{num(1, "")}},
		{name: "operators", in: "(),*/", want: []cssToken{opTok('('), opTok(')'), opTok(','), opTok('*'), opTok('/')}},
		{name: "operators with spaces", in: "( 1 , 2 ) * 3 / 4", want: []cssToken{
			opTok('('), num(1, ""), opTok(','), num(2, ""), opTok(')'), opTok('*'), num(3, ""), opTok('/'), num(4, "")}},

		// A sign is part of the number unless the previous token is a value.
		{name: "binary minus after a number", in: "10px-2px", want: []cssToken{num(10, "px"), opTok('-'), num(2, "px")}},
		{name: "binary plus after a number", in: "10px+2px", want: []cssToken{num(10, "px"), opTok('+'), num(2, "px")}},
		{name: "binary then sign", in: "10px - -2px", want: []cssToken{num(10, "px"), opTok('-'), num(-2, "px")}},
		{name: "binary plus with spaces", in: "1 + 5", want: []cssToken{num(1, ""), opTok('+'), num(5, "")}},
		{name: "binary plus glued to the right", in: "1 +5", want: []cssToken{num(1, ""), opTok('+'), num(5, "")}},
		{name: "leading minus", in: "-5px", want: []cssToken{num(-5, "px")}},
		{name: "leading plus and dot", in: "+.5em", want: []cssToken{num(0.5, "em")}},
		{name: "leading dot", in: ".5", want: []cssToken{num(0.5, "")}},
		{name: "minus after an identifier", in: "a -1", want: []cssToken{id("a"), opTok('-'), num(1, "")}},
		{name: "minus after a parenthesis", in: "(1)-2", want: []cssToken{opTok('('), num(1, ""), opTok(')'), opTok('-'), num(2, "")}},
		{name: "sign after an open parenthesis", in: "(-1)", want: []cssToken{opTok('('), num(-1, ""), opTok(')')}},
		{name: "sign after a comma", in: ",-3px", want: []cssToken{opTok(','), num(-3, "px")}},
		{name: "sign after an operator", in: "*-3", want: []cssToken{opTok('*'), num(-3, "")}},
		{name: "minus before an open parenthesis", in: "-(", want: []cssToken{opTok('-'), opTok('(')}},
		{name: "minus before an identifier", in: "-a", want: []cssToken{opTok('-'), id("a")}},
		{name: "double minus", in: "--1", want: []cssToken{opTok('-'), num(-1, "")}},
		{name: "lone minus", in: "-", want: []cssToken{opTok('-')}},
		{name: "lone plus", in: "+", want: []cssToken{opTok('+')}},
		{name: "minus before a space and a digit", in: "- 1", want: []cssToken{opTok('-'), num(1, "")}},
		{name: "var-like", in: "var(--x)", want: []cssToken{id("var"), opTok('('), opTok('-'), opTok('-'), id("x"), opTok(')')}},

		// Numbers.
		{name: "unit", in: "1.5rem", want: []cssToken{num(1.5, "rem")}},
		{name: "percent", in: "50%", want: []cssToken{num(50, "%")}},
		{name: "unitless", in: "2", want: []cssToken{num(2, "")}},
		{name: "uppercase unit", in: "3PX", want: []cssToken{num(3, "PX")}},
		{name: "unit stops at a digit", in: "1e3", want: []cssToken{num(1, "e"), num(3, "")}},
		{name: "percent ends the unit", in: "5%px", want: []cssToken{num(5, "%"), id("px")}},
		{name: "number then identifier", in: "2 a", want: []cssToken{num(2, ""), id("a")}},

		// Identifiers.
		{name: "identifier with dash", in: "a-b", want: []cssToken{id("a-b")}},
		{name: "identifier with digits", in: "x1_y-2", want: []cssToken{id("x1_y-2")}},
		{name: "identifier from underscore", in: "_a", want: []cssToken{id("_a")}},
		{name: "identifier with a trailing dash", in: "a-", want: []cssToken{id("a-")}},
		{name: "identifier with an uppercase letter", in: "Ab", want: []cssToken{id("Ab")}},
		{name: "function", in: "calc(1px+2px)", want: []cssToken{
			id("calc"), opTok('('), num(1, "px"), opTok('+'), num(2, "px"), opTok(')')}},
		{name: "min", in: "min(10px,5%)", want: []cssToken{
			id("min"), opTok('('), num(10, "px"), opTok(','), num(5, "%"), opTok(')')}},

		// Errors.
		{name: "two dots", in: "1.2.3", err: `invalid number "1.2.3"`},
		{name: "lone dot", in: ".", err: `invalid number "."`},
		{name: "signed dot", in: "-.", err: `invalid number "-."`},
		{name: "number error after tokens", in: "(1 + 2.3.4)", err: `invalid number "2.3.4"`},
		{name: "hash", in: "#", err: `unexpected character "#"`},
		{name: "dollar after a number", in: "1 $", err: `unexpected character "$"`},
		{name: "double percent", in: "1%%", err: `unexpected character "%"`},
		{name: "semicolon", in: "1px;", err: `unexpected character ";"`},
		{name: "non-ASCII byte", in: "é", err: "unexpected character \"Ã\""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := lexCSS(tc.in)
			if tc.err != "" {
				if err == nil || err.Error() != tc.err {
					t.Fatalf("lexCSS(%q) error = %v, want %q", tc.in, err, tc.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("lexCSS(%q) error = %v", tc.in, err)
			}
			if !slices.Equal(got, tc.want) {
				t.Errorf("lexCSS(%q) = %+v, want %+v", tc.in, got, tc.want)
			}
		})
	}
}
