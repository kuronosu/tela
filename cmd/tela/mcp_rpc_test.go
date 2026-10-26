package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

// fakeTools is an mcpTools with one tool, echo, that answers its
// arguments, and fails when they say so.
type fakeTools struct{}

func (fakeTools) list() []mcpTool {
	return []mcpTool{{Name: "echo", Description: "Echo the arguments. Example: {}", InputSchema: json.RawMessage(`{"type":"object","properties":{}}`)}}
}

func (fakeTools) call(_ context.Context, name string, args json.RawMessage) (mcpResult, bool) {
	if name != "echo" {
		return mcpResult{}, false
	}
	return mcpResult{Content: []mcpContent{{Type: "text", Text: string(args)}}, IsError: strings.Contains(string(args), "fail")}, true
}

func (fakeTools) instructions() string { return "be nice" }

// rpcExchange serves the lines of input on a server with fakeTools and
// returns the messages it wrote, one per line, decoded.
func rpcExchange(t *testing.T, input ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	s := &rpcServer{tools: fakeTools{}, name: "tela", version: "v0.0.0-test", logf: func(string, ...any) {}, out: &out}
	if err := s.serve(context.Background(), strings.NewReader(strings.Join(input, "\n")+"\n")); err != nil {
		t.Fatal(err)
	}
	var msgs []map[string]any
	sc := bufio.NewScanner(&out)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "[") {
			var batch []map[string]any
			if err := json.Unmarshal([]byte(line), &batch); err != nil {
				t.Fatalf("output line is not JSON: %q", line)
			}
			msgs = append(msgs, map[string]any{"batch": batch})
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("output line is not a JSON object: %q", line)
		}
		if m["jsonrpc"] != "2.0" {
			t.Errorf("response without jsonrpc 2.0: %q", line)
		}
		msgs = append(msgs, m)
	}
	return msgs
}

// byID returns the response with the given id (a JSON number).
func byID(t *testing.T, msgs []map[string]any, id float64) map[string]any {
	t.Helper()
	for _, m := range msgs {
		if m["id"] == id {
			return m
		}
	}
	t.Fatalf("no response with id %v in %v", id, msgs)
	return nil
}

func errorCode(m map[string]any) float64 {
	e, _ := m["error"].(map[string]any)
	if e == nil {
		return 0
	}
	return e["code"].(float64)
}

func TestMCPInitializeNegotiatesTheVersion(t *testing.T) {
	for _, tc := range []struct{ asked, want string }{
		{"2025-06-18", "2025-06-18"},
		{"2024-11-05", "2024-11-05"},
		{"2025-03-26", "2025-03-26"},
		{"2025-11-25", "2025-11-25"},
		{"1999-01-01", mcpProtocolVersions[0]},
		{"", mcpProtocolVersions[0]},
	} {
		msgs := rpcExchange(t, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"`+tc.asked+`","capabilities":{},"clientInfo":{"name":"t","version":"1"}}}`)
		if len(msgs) != 1 {
			t.Fatalf("%d messages for one initialize", len(msgs))
		}
		res, _ := msgs[0]["result"].(map[string]any)
		if res == nil {
			t.Fatalf("initialize %q: no result: %v", tc.asked, msgs[0])
		}
		if res["protocolVersion"] != tc.want {
			t.Errorf("initialize %q answered %v, want %s", tc.asked, res["protocolVersion"], tc.want)
		}
		info, _ := res["serverInfo"].(map[string]any)
		if info["name"] != "tela" || info["version"] != "v0.0.0-test" {
			t.Errorf("serverInfo = %v", info)
		}
		caps, _ := res["capabilities"].(map[string]any)
		if _, ok := caps["tools"].(map[string]any); !ok {
			t.Errorf("capabilities = %v, want tools", caps)
		}
		if res["instructions"] != "be nice" {
			t.Errorf("instructions = %v", res["instructions"])
		}
	}
}

func TestMCPNotificationsGetNoResponse(t *testing.T) {
	msgs := rpcExchange(t,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":3}}`,
		`{"jsonrpc":"2.0","method":"some/unknown/notification"}`,
		`{"jsonrpc":"2.0","id":7,"result":{}}`, // a response from the client
		`{"jsonrpc":"2.0","id":9,"method":"ping"}`,
	)
	if len(msgs) != 1 {
		t.Fatalf("got %d messages, want only the ping's: %v", len(msgs), msgs)
	}
	if msgs[0]["id"] != 9.0 || msgs[0]["result"] == nil {
		t.Errorf("ping answered %v, want an empty result", msgs[0])
	}
	if r, _ := msgs[0]["result"].(map[string]any); len(r) != 0 {
		t.Errorf("ping result %v, want {}", r)
	}
}

func TestMCPErrors(t *testing.T) {
	msgs := rpcExchange(t,
		`{not json`,
		`42`,
		`{"jsonrpc":"1.0","id":2,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":3}`,
		`{"jsonrpc":"2.0","id":4,"method":"resources/list"}`,
		`{"jsonrpc":"2.0","id":null,"method":"ping"}`,
		`{"jsonrpc":"2.0","id":{"a":1},"method":"ping"}`,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"nope","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":[1]}`,
		`{"jsonrpc":"2.0","id":8,"method":7}`,
		`[]`,
	)
	var nulls []float64
	for _, m := range msgs {
		if m["id"] == nil {
			nulls = append(nulls, errorCode(m))
		}
	}
	// {not json is a parse error; 42, a null id, an object id and the
	// empty batch are invalid requests without an id to answer.
	slices.Sort(nulls)
	if want := []float64{rpcParseError, rpcInvalidRequest, rpcInvalidRequest, rpcInvalidRequest, rpcInvalidRequest}; !slices.Equal(nulls, want) {
		t.Errorf("errors without id: %v, want %v", nulls, want)
	}
	for id, code := range map[float64]float64{
		2: rpcInvalidRequest, // jsonrpc 1.0
		3: rpcInvalidRequest, // no method
		4: rpcMethodNotFound,
		5: rpcInvalidParams, // unknown tool
		6: rpcInvalidParams, // no tool name
		7: rpcInvalidParams, // params not an object
		8: rpcInvalidRequest,
	} {
		if got := errorCode(byID(t, msgs, id)); got != code {
			t.Errorf("request %v: error %v, want %v", id, got, code)
		}
	}
}

func TestMCPToolsCall(t *testing.T) {
	msgs := rpcExchange(t,
		`{"jsonrpc":"2.0","id":"a","method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"echo","arguments":{"x":1}}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/call","params":{"name":"echo","arguments":{"fail":true}}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"echo"}}`,
	)
	var list map[string]any
	for _, m := range msgs {
		if m["id"] == "a" {
			list = m
		}
	}
	tools, _ := list["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "echo" {
		t.Errorf("tools/list = %v", list)
	}
	ok := byID(t, msgs, 1)["result"].(map[string]any)
	if ok["isError"] != nil || ok["content"].([]any)[0].(map[string]any)["text"] != `{"x":1}` {
		t.Errorf("tools/call echo = %v", ok)
	}
	fail := byID(t, msgs, 2)["result"].(map[string]any)
	if fail["isError"] != true {
		t.Errorf("a failing tool did not set isError: %v", fail)
	}
	if byID(t, msgs, 3)["result"] == nil {
		t.Errorf("a call without arguments: %v", byID(t, msgs, 3))
	}
}

func TestMCPBatch(t *testing.T) {
	msgs := rpcExchange(t, `[{"jsonrpc":"2.0","id":1,"method":"ping"},{"jsonrpc":"2.0","method":"notifications/initialized"},{"jsonrpc":"2.0","id":2,"method":"nope"}]`)
	if len(msgs) != 1 {
		t.Fatalf("%d messages for a batch, want one array", len(msgs))
	}
	batch, _ := msgs[0]["batch"].([]map[string]any)
	if len(batch) != 2 || batch[0]["id"] != 1.0 || errorCode(batch[1]) != rpcMethodNotFound {
		t.Errorf("batch response = %v", msgs[0])
	}
}

// TestMCPToolSchemas checks that every tool of tela mcp has a description
// with an example and a valid object schema whose required arguments are
// among its properties, and that the argument check knows them.
func TestMCPToolSchemas(t *testing.T) {
	defs := mcpToolDefs([]string{"Login", "Settings"})
	var names []string
	for _, d := range defs {
		tool := d.tool
		names = append(names, tool.Name)
		if !strings.Contains(tool.Description, "Example: {") {
			t.Errorf("%s: the description has no example: %q", tool.Name, tool.Description)
		}
		var schema struct {
			Type       string                    `json:"type"`
			Properties map[string]map[string]any `json:"properties"`
			Required   []string                  `json:"required"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("%s: inputSchema is not JSON: %v", tool.Name, err)
		}
		if schema.Type != "object" || schema.Properties == nil {
			t.Errorf("%s: inputSchema %s is not an object schema with properties", tool.Name, tool.InputSchema)
		}
		for name, p := range schema.Properties {
			switch p["type"] {
			case "string", "number", "integer", "boolean", "object":
			default:
				t.Errorf("%s.%s: type %v", tool.Name, name, p["type"])
			}
			if p["description"] == "" || p["description"] == nil {
				t.Errorf("%s.%s has no description", tool.Name, name)
			}
			if !slices.Contains(d.props, name) {
				t.Errorf("%s.%s is not in the argument check", tool.Name, name)
			}
		}
		for _, r := range schema.Required {
			if _, ok := schema.Properties[r]; !ok {
				t.Errorf("%s: required %q is not a property", tool.Name, r)
			}
		}
	}
	want := []string{"snapshot", "screenshot", "click", "hover", "fill", "press", "scroll", "drag", "resize", "open", "wait", "inspect", "lint"}
	if !slices.Equal(names, want) {
		t.Errorf("tools %v, want the set of ADR-174 %v", names, want)
	}
	for _, d := range defs {
		if d.tool.Name == "open" && !strings.Contains(d.tool.Description, "Login, Settings") {
			t.Errorf("open does not list the previews: %q", d.tool.Description)
		}
	}
}

func TestMCPUnknownArgument(t *testing.T) {
	for _, d := range mcpToolDefs(nil) {
		if d.tool.Name != "click" {
			continue
		}
		if err := checkArgNames(&d, json.RawMessage(`{"ref":"e1","element":"the Save button"}`)); err != nil {
			t.Errorf("ref and element: %v", err)
		}
		err := checkArgNames(&d, json.RawMessage(`{"reff":"e1"}`))
		if err == nil || !strings.Contains(err.Error(), `unknown argument "reff"`) || !strings.Contains(err.Error(), "ref, role") {
			t.Errorf("unknown argument: %v", err)
		}
	}
}

func TestParseMCPArgs(t *testing.T) {
	o, err := parseMCPArgs(nil)
	if err != nil || o.Pkg != "." || o.W != defaultWidth || o.H != defaultHeight || o.Scale != 1 || o.SizeSet {
		t.Errorf("defaults: %+v, %v", o, err)
	}
	o, err = parseMCPArgs([]string{"./app", "--size", "390x844", "--scale", "2", "--dark", "--preview", "Login", "-v"})
	if err != nil || o.Pkg != "./app" || o.W != 390 || o.H != 844 || !o.SizeSet || o.Scale != 2 || !o.Dark || o.Preview != "Login" || !o.Verbose {
		t.Errorf("flags: %+v, %v", o, err)
	}
	for _, bad := range [][]string{{"a", "b"}, {"--size", "x"}, {"--scale", "0"}, {"--nope"}} {
		if _, err := parseMCPArgs(bad); err == nil {
			t.Errorf("%v: no error", bad)
		}
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"help", "mcp"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "tela mcp [--size WxH]") {
		t.Errorf("help mcp: code %d, %q", code, out.String())
	}
}
