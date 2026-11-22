package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// The check and verify tools of tela mcp (ADR-312): the commands of the same
// names, run on the package the server serves. They mount the package's
// screens in a process of their own, as the commands do, so what they find
// does not depend on the state of the live instance; they read the source as
// it is now.

var checkProps = []schemaProp{
	{"preview", "string", "Only this preview of the package's Previews (check: the first by default; verify: all by default)", nil},
	{"rules", "string", "Only these rules, comma separated, such as \"color-contrast,text-clipped\" (default: all)", nil},
	{"disable", "string", "Leave these rules out, comma separated", nil},
}

var verifyProps = append(append([]schemaProp(nil), checkProps...),
	schemaProp{"widths", "string", `Widths to try, comma separated (default "360,768,1280")`, nil},
	schemaProp{"textScale", "number", "Text scale of the large text variant, above 1 and at most 4 (default 2)", nil},
	schemaProp{"lang", "string", "Locale of the language variant, a BCP 47 tag (default: the first other language of the package's tela/i18n catalogs)", nil},
)

const (
	checkDesc  = `Look for what a designer would call a bug on the served screen, in light and dark mode: text that is cut off or lies over other text, targets over targets, low contrast (WCAG 2.2 AA), targets under 24x24, controls a keyboard cannot operate, focus that shows nothing, and missing names and states for screen readers. Each finding has its rule, the element's aria line, its [src=file:line] and the fix. It reads the source as it is, in a process of its own (the live screen's state does not matter). Run verify for every preview and a matrix of widths, large text, right-to-left and another language. Example: {} or {"rules": "color-contrast"}`
	verifyDesc = `check over every preview, in light and dark, at several widths, with large text, right-to-left and in another language. Each finding lists the variants it appears in ("Settings / width 360, text x2"). Slower than check (a mount per preview and variant). Example: {} or {"preview": "Login", "widths": "320,480"}`
)

// mcpCheckDefs are the check and verify tools.
func mcpCheckDefs() []mcpToolDef {
	ro := func(title string) *mcpToolAnnotate {
		return &mcpToolAnnotate{Title: title, ReadOnlyHint: true, IdempotentHint: true}
	}
	return []mcpToolDef{
		{tool: mcpTool{Name: "check", Description: checkDesc, InputSchema: schemaOf(checkProps), Annotations: ro("Check the screen")},
			props: propNames(checkProps), run: func(s *mcpSession, ctx context.Context, args json.RawMessage) mcpResult {
				return s.toolCheck(false, args)
			}},
		{tool: mcpTool{Name: "verify", Description: verifyDesc, InputSchema: schemaOf(verifyProps), Annotations: ro("Verify every screen")},
			props: propNames(verifyProps), run: func(s *mcpSession, ctx context.Context, args json.RawMessage) mcpResult {
				return s.toolCheck(true, args)
			}},
	}
}

func (s *mcpSession) toolCheck(verify bool, raw json.RawMessage) mcpResult {
	var o mcpOut
	var a struct {
		Preview   string  `json:"preview"`
		Rules     string  `json:"rules"`
		Disable   string  `json:"disable"`
		Widths    string  `json:"widths"`
		TextScale float64 `json:"textScale"`
		Lang      string  `json:"lang"`
	}
	if err := decodeArgs(raw, &a); err != nil {
		o.fail(err.Error())
		return o.result()
	}
	cmd := []string{"--preview", a.Preview, "--rules", a.Rules, "--disable", a.Disable}
	if verify {
		cmd = append(cmd, "--widths", a.Widths, "--lang", a.Lang)
		if a.TextScale != 0 {
			cmd = append(cmd, "--text-scale", fmt.Sprint(a.TextScale))
		}
	}
	// Empty flag values mean the defaults.
	var args []string
	for i := 0; i < len(cmd); i += 2 {
		if cmd[i+1] != "" {
			args = append(args, cmd[i], cmd[i+1])
		}
	}
	name := "check"
	if verify {
		name = "verify"
	}
	opts, err := parseCheckArgs(verify, append(args, s.pkgArg))
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var logs bytes.Buffer
	rep, err := checkPackage(opts, &logs)
	if err != nil {
		o.fail(err.Error())
		return o.result()
	}
	var b bytes.Buffer
	writeCheckReport(&b, name, rep)
	o.head = strings.TrimRight(b.String(), "\n")
	if n := strings.TrimSpace(logs.String()); n != "" {
		o.other = appendNew(o.other, n)
	}
	return o.result()
}
