package main

import (
	"context"
	"encoding/json"
	"slices"
)

// film and test are not available over the real windows of tela mcp
// --attach yet: film needs the headless app's controllable clock, and test
// replays a session on a headless mount. The attached server lists them
// after its own tools, in the order of the headless list, and says why when
// they are called.
func init() {
	for _, t := range []struct {
		name, desc, why string
		props           []schemaProp
		required        string
	}{
		{"film", filmDesc, "film needs the app's controllable clock, which only the headless server (tela mcp ./pkg) has: a real window runs on the wall clock. Use tela snapshot --film, or the headless server", filmProps, "span"},
		{"test", testDesc, "test writes the session of the headless server (tela mcp ./pkg) as a telatest test, which mounts the screen headlessly; a session on a real window is not recorded yet", testProps, "name"},
	} {
		props := append(slices.Clone(t.props), windowProp)
		attachDefs = append(attachDefs, attachToolDef{
			mcpToolDef: mcpToolDef{tool: mcpTool{Name: t.name, Description: "Not available while attached to tela dev. " + t.desc, InputSchema: schemaOf(props, t.required), Annotations: &mcpToolAnnotate{Title: t.name}}, props: propNames(props)},
			run: func(*attachSession, context.Context, json.RawMessage) mcpResult {
				var o mcpOut
				o.fail(t.name + " is not available while attached to tela dev: " + t.why)
				return o.result()
			},
		})
	}
}
