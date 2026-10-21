//go:build ignore

// gen_deprecated writes deprecated.json, the list of the names Lucide keeps
// for icons it renamed, from the lucide-static package:
//
//	go run gen_deprecated.go /path/to/package/icons
//
// The argument is the icons directory of the unpacked package (the same
// value as $LUCIDE_ICONS in gen.go). Lucide's renamed icons stay in the
// package as extra SVG files with the old name, and the package says which
// icon replaces each one only in its ES module index (dist/esm/lucide-static.mjs),
// as lines like
//
//	export { default as AlertCircle, default as CircleAlert } from './icons/circle-alert.mjs';
//
// An SVG file that is not a key of icon-nodes.json (the list of the current
// icons) is an old name; the line that exports its PascalCase name says
// which file draws it now. The output maps the old file name to the current
// one ({"alert-circle": "circle-alert"}), the format of "tela icons gen
// -deprecated". It is the same on every run.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	if len(os.Args) != 2 {
		fail("usage: go run gen_deprecated.go <lucide-static>/icons")
	}
	icons := os.Args[1]
	pkg := filepath.Dir(filepath.Clean(icons))

	var current map[string]json.RawMessage
	readJSON(filepath.Join(pkg, "icon-nodes.json"), &current)

	index, err := os.ReadFile(filepath.Join(pkg, "dist", "esm", "lucide-static.mjs"))
	if err != nil {
		fail("%v", err)
	}
	line := regexp.MustCompile(`(?m)^export \{ (.*) \} from '\./icons/(.*)\.mjs';$`)
	name := regexp.MustCompile(`default as (\w+)`)
	fileOf := map[string]string{} // PascalCase export name -> file that draws it
	for _, m := range line.FindAllStringSubmatch(string(index), -1) {
		for _, n := range name.FindAllStringSubmatch(m[1], -1) {
			fileOf[n[1]] = m[2]
		}
	}

	files, err := filepath.Glob(filepath.Join(icons, "*.svg"))
	if err != nil {
		fail("%v", err)
	}
	out := map[string]string{}
	for _, f := range files {
		base := strings.TrimSuffix(filepath.Base(f), ".svg")
		if _, ok := current[base]; ok {
			continue
		}
		target, ok := fileOf[pascal(base)]
		if !ok {
			fail("%s.svg is not a current icon and lucide-static.mjs has no export %s for it", base, pascal(base))
		}
		if _, ok := current[target]; !ok {
			fail("%s.svg: its replacement %q is not a current icon", base, target)
		}
		out[base] = target
	}
	if len(out) == 0 {
		fail("no old names found in %s: is it the icons directory of lucide-static?", icons)
	}
	data, err := json.MarshalIndent(out, "", "\t") // keys are sorted
	if err != nil {
		fail("%v", err)
	}
	if err := os.WriteFile("deprecated.json", append(data, '\n'), 0o644); err != nil {
		fail("%v", err)
	}
	fmt.Printf("gen_deprecated: wrote deprecated.json: %d names\n", len(out))
}

// pascal is how lucide-static spells a file name as an export: "alert-circle"
// is "AlertCircle", "axis-3-d" is "Axis3D".
func pascal(file string) string {
	var sb strings.Builder
	for _, p := range strings.Split(file, "-") {
		if p != "" {
			sb.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return sb.String()
}

func readJSON(path string, v any) {
	data, err := os.ReadFile(path)
	if err != nil {
		fail("%v", err)
	}
	if err := json.Unmarshal(data, v); err != nil {
		fail("%s: %v", path, err)
	}
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "gen_deprecated: "+format+"\n", args...)
	os.Exit(1)
}
