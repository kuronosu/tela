package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"strings"
	"testing"
)

func TestParseCheckArgs(t *testing.T) {
	o, err := parseCheckArgs(false, []string{"--preview", "Login", "-json", "./x"})
	if err != nil || o.Pkg != "./x" || o.Preview != "Login" || !o.JSON || o.Verify {
		t.Errorf("check: %+v, %v", o, err)
	}
	if vs := o.variants(); len(vs) != 2 || vs[0].Name != "light" || vs[1].Name != "dark" || !vs[1].Dark {
		t.Errorf("check variants: %+v", vs)
	}
	o, err = parseCheckArgs(true, []string{"--widths", "400, 900", "--text-scale", "1.5", "--lang", "es", "--rules", "color-contrast", "./x"})
	if err != nil || len(o.Widths) != 2 || o.TextScale != 1.5 || o.Lang != "es" {
		t.Fatalf("verify: %+v, %v", o, err)
	}
	var names []string
	for _, v := range o.variants() {
		names = append(names, v.Name)
	}
	if got := strings.Join(names, "|"); got != "light|dark|width 400|width 900|text x1.5|rtl|lang" {
		t.Errorf("verify variants: %s", got)
	}
	for _, v := range o.variants() {
		if v.Focus != (v.Name == "light" || v.Name == "dark") {
			t.Errorf("variant %s focus %v", v.Name, v.Focus)
		}
	}
	for args, want := range map[string]string{
		"--rules nope ./x":     `unknown rule "nope"`,
		"--disable a,b ./x":    `unknown rule "a"`,
		"--size 0x1 ./x":       "invalid --size",
		"":                     "missing <pkg>",
		"--widths 20 ./x":      "invalid --widths",
		"--text-scale 9 ./x":   "invalid --text-scale",
		"--text-scale 0.5 ./x": "invalid --text-scale",
		"--widths nope ./x":    "invalid --widths",
	} {
		if _, err := parseCheckArgs(true, strings.Fields(args)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("verify %q: %v, want %q", args, err, want)
		}
	}
	// check has no verify flags.
	if _, err := parseCheckArgs(false, []string{"--widths", "400", "./x"}); err == nil {
		t.Error("tela check took --widths")
	}
	if _, err := parseCheckArgs(false, []string{"-h"}); !errors.Is(err, flag.ErrHelp) {
		t.Errorf("-h: %v", err)
	}
}

func TestCheckDispatch(t *testing.T) {
	for _, cmd := range []string{"check", "verify"} {
		var out, errOut bytes.Buffer
		if code := run([]string{cmd, "-h"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "tela "+cmd) {
			t.Errorf("%s -h: %d %q", cmd, code, out.String())
		}
		out.Reset()
		if code := run([]string{"help", cmd}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "Usage:") {
			t.Errorf("help %s: %d", cmd, code)
		}
		if code := run([]string{cmd}, &out, &errOut); code != 2 {
			t.Errorf("%s without a package: %d", cmd, code)
		}
	}
}

func TestMergeCheck(t *testing.T) {
	var raw checkRaw
	js := `{"names":["A","B"],"results":[
	 {"preview":"A","variant":"light","violations":[{"rule":"text-clipped","impact":"moderate","message":"cut 10 px","fix":"f","element":"text \"x\"","src":"a.go:3"}]},
	 {"preview":"A","variant":"width 360","violations":[{"rule":"text-clipped","impact":"moderate","message":"cut 90 px","fix":"f","element":"text \"x\"","src":"a.go:3"}]},
	 {"preview":"A","variant":"dark","violations":[{"rule":"color-contrast","impact":"serious","message":"text is #aaaaaa on #000000: 2.0:1","fix":"f","element":"text \"y\"","src":"a.go:4"}]},
	 {"preview":"A","variant":"light","violations":[{"rule":"color-contrast","impact":"serious","message":"text is #bbbbbb on #ffffff: 3.0:1","fix":"f","element":"text \"y\"","src":"a.go:4"}]},
	 {"preview":"B","variant":"dark","skipped":"fixed"}]}`
	if err := json.Unmarshal([]byte(js), &raw); err != nil {
		t.Fatal(err)
	}
	rep := mergeCheck("./p", &raw, checkSpec{Variants: []checkVariant{{Name: "light"}, {Name: "dark"}}})
	if len(rep.Findings) != 3 {
		t.Fatalf("findings: %+v", rep.Findings)
	}
	if f := rep.Findings[0]; f.Rule != "text-clipped" || strings.Join(f.Variants, ",") != "light,width 360" || f.Message != "cut 10 px" {
		t.Errorf("merged: %+v", f)
	}
	if len(rep.Skipped) != 1 || rep.Skipped[0] != "B / dark: fixed" {
		t.Errorf("skipped: %v", rep.Skipped)
	}
	var out bytes.Buffer
	writeCheckReport(&out, "verify", rep)
	for _, want := range []string{"a.go:3: text-clipped (moderate): cut 10 px", "in: A / light, width 360", "skipped: B / dark: fixed", "tela verify: 3 findings in ./p"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("report lacks %q:\n%s", want, out.String())
		}
	}
}
