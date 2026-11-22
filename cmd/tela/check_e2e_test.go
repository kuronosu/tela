package main

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

// checkRun runs tela check or verify on the test screens and returns the
// exit code, the parsed JSON report and what it printed to standard error.
func checkRun(t *testing.T, cmd string, args ...string) (int, *checkReport, string) {
	t.Helper()
	var out, errOut bytes.Buffer
	code := run(append([]string{cmd, "-json"}, append(args, "./testdata/checkscreens")...), &out, &errOut)
	var rep checkReport
	if code <= 1 {
		if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
			t.Fatalf("tela %s %v: exit %d, not JSON: %v\nstdout: %s\nstderr: %s", cmd, args, code, err, out.String(), errOut.String())
		}
	}
	return code, &rep, errOut.String()
}

// findings returns the findings of a rule in a preview.
func (r *checkReport) findings(rule, preview string) []checkFinding {
	var out []checkFinding
	for _, f := range r.Findings {
		if f.Rule == rule && f.Preview == preview {
			out = append(out, f)
		}
	}
	return out
}

func TestCheckFindsAndNamesTheElements(t *testing.T) {
	skipShort(t)
	code, rep, _ := checkRun(t, "check", "--preview", "Broken")
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	for _, c := range []struct{ rule, element string }{
		{"aria-command-name", "button"},
		{"color-contrast", `text "Quarterly revenue"`},
		{"text-clipped", `text "A very long caption`},
		{"focus-visible", `button "Cancel"`},
	} {
		fs := rep.findings(c.rule, "Broken")
		if len(fs) != 1 || !strings.Contains(fs[0].Element, c.element) || !strings.HasPrefix(fs[0].Src, "checkscreens.go:") || fs[0].Fix == "" {
			t.Errorf("%s: %+v", c.rule, fs)
		}
	}
	if strings.Join(rep.Variants, ",") != "light,dark" {
		t.Errorf("variants %v", rep.Variants)
	}
	// Light and dark share the findings: each is one.
	for _, f := range rep.Findings {
		if strings.Join(f.Variants, ",") != "light,dark" {
			t.Errorf("%s in %v", f.Rule, f.Variants)
		}
	}
}

func TestCheckCleanScreenExitsZero(t *testing.T) {
	skipShort(t)
	code, rep, _ := checkRun(t, "check", "--preview", "Clean")
	if code != 0 || len(rep.Findings) != 0 {
		t.Errorf("exit %d, findings %+v", code, rep.Findings)
	}
	var out, errOut bytes.Buffer
	if code := run([]string{"check", "--preview", "Clean", "./testdata/checkscreens"}, &out, &errOut); code != 0 || !strings.Contains(out.String(), "nothing found") {
		t.Errorf("text: exit %d, %q", code, out.String())
	}
}

func TestCheckRulesAndDisable(t *testing.T) {
	skipShort(t)
	_, rep, _ := checkRun(t, "check", "--preview", "Broken", "--rules", "color-contrast,text-clipped")
	if len(rep.Findings) != 2 {
		t.Errorf("--rules: %+v", rep.Findings)
	}
	_, rep, _ = checkRun(t, "check", "--preview", "Broken", "--disable", "focus-visible,aria-command-name,color-contrast")
	if len(rep.Findings) != 1 || rep.Findings[0].Rule != "text-clipped" {
		t.Errorf("--disable: %+v", rep.Findings)
	}
}

// TestVerifyMatrix: a problem that only a narrower window, large text or a
// language shows is found by verify and named with its variant.
func TestVerifyMatrix(t *testing.T) {
	skipShort(t)
	code, rep, _ := checkRun(t, "verify")
	if code != 1 {
		t.Fatalf("exit code %d, want 1", code)
	}
	if strings.Join(rep.Previews, ",") != "Broken,Clean,Narrow,Scaled" {
		t.Errorf("previews %v", rep.Previews)
	}
	if len(rep.Variants) != 8 {
		t.Errorf("variants %v", rep.Variants)
	}
	clipped := rep.findings("text-clipped", "Narrow")
	if len(clipped) != 1 || strings.Join(clipped[0].Variants, ",") != "width 360" {
		t.Errorf("Narrow is clipped only at 360: %+v", clipped)
	}
	scaled := rep.findings("text-clipped", "Scaled")
	if len(scaled) != 1 || strings.Join(scaled[0].Variants, ",") != "text x2" {
		t.Errorf("Scaled is clipped only with large text: %+v", scaled)
	}
	if fs := rep.findings("text-clipped", "Broken"); len(fs) != 1 || len(fs[0].Variants) != 7 {
		t.Errorf("Broken's caption is clipped everywhere: %+v", fs)
	}
	for _, f := range rep.Findings {
		if f.Preview == "Clean" {
			t.Errorf("the clean screen has a finding: %+v", f)
		}
	}
	// This package has no tela/i18n: the language variant says so, once.
	if len(rep.Skipped) != 1 || !strings.Contains(rep.Skipped[0], "lang: the package does not import tela/i18n") {
		t.Errorf("skipped %v", rep.Skipped)
	}
	// A narrower list of widths.
	_, rep, _ = checkRun(t, "verify", "--preview", "Narrow", "--widths", "400")
	if got := strings.Join(rep.Variants, ","); got != "light,dark,width 400,text x2,rtl,lang" {
		t.Errorf("variants %v", got)
	}
	if fs := rep.findings("text-clipped", "Narrow"); len(fs) != 1 || strings.Join(fs[0].Variants, ",") != "width 400" {
		t.Errorf("Narrow at 400: %+v", fs)
	}
}

func TestCheckErrors(t *testing.T) {
	skipShort(t)
	var out, errOut bytes.Buffer
	if code := run([]string{"check", "--preview", "Nope", "./testdata/checkscreens"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), `unknown preview "Nope"`) {
		t.Errorf("unknown preview: %d %s", code, errOut.String())
	}
	errOut.Reset()
	if code := run([]string{"check", "./testdata/empty"}, &out, &errOut); code != 2 || !strings.Contains(errOut.String(), "func Preview() tela.Node") {
		t.Errorf("no screen: %d %s", code, errOut.String())
	}
}
