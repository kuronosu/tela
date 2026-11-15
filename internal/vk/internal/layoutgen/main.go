// Command layoutgen checks the hand-written Vulkan bindings of internal/vk
// against the real Vulkan headers. It is a development tool, run by
// `go generate ./internal/vk` on a machine with the headers and a C
// compiler; nothing builds or runs it otherwise.
//
// It reads structs.go and enums.go of the package (as source, so it needs
// no build tag) and vulkan_core.h, and writes two files:
//
//   - internal/layoutgen/c/layout.c, a C program that prints the size and
//     alignment of every struct of structs.go and the offset and size of
//     every member the header gives it, and the value of every constant
//     of enums.go, as Go source;
//   - layout_table_test.go, the output of that program compiled against
//     the headers and run, which TestLayout compares with unsafe.Sizeof,
//     unsafe.Alignof and unsafe.Offsetof of the Go types. The file states
//     the same layouts a second time as constant expressions that do not
//     compile when one differs, so cross-compiling the tests (go vet with
//     GOOS and GOARCH) checks the Go side of the targets whose tests do
//     not run.
//
// The member lists come from the header, not from the Go structs, so a
// missing, misplaced or misnamed field is found here or by the test, and
// not only when a size happens to change. A Go type of structs.go is the
// C type named Vk plus its name; a field is the member of the same name
// with a lower-case first letter; a constant names its C constant in its
// trailing comment.
//
// With -cross it also checks, with clang and no system headers, that the
// C layouts are the same on linux/arm64, windows/amd64 and windows/arm64
// as on the machine the table was made on, which is what lets one table
// stand for all four targets.
//
// The headers are looked for in -include, $TELA_VULKAN_INCLUDE,
// ~/.cache/tela-vulkan-dev/root/usr/include and /usr/include.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

func main() {
	log.SetFlags(0)
	log.SetPrefix("layoutgen: ")
	include := flag.String("include", "", "directory that holds vulkan/vulkan_core.h")
	cc := flag.String("cc", "cc", "C compiler")
	pkg := flag.String("pkg", ".", "directory of the package internal/vk")
	cross := flag.Bool("cross", false, "also check the layouts of the other targets with clang")
	flag.Parse()

	dir, err := includeDir(*include)
	if err != nil {
		log.Fatal(err)
	}
	header, err := os.ReadFile(filepath.Join(dir, "vulkan", "vulkan_core.h"))
	if err != nil {
		log.Fatal(err)
	}
	types, err := goTypes(filepath.Join(*pkg, "structs.go"))
	if err != nil {
		log.Fatal(err)
	}
	consts, err := goConsts(filepath.Join(*pkg, "enums.go"))
	if err != nil {
		log.Fatal(err)
	}
	for i := range types {
		if err := types[i].readC(string(header)); err != nil {
			log.Fatal(err)
		}
	}

	cPath := filepath.Join(*pkg, "internal", "layoutgen", "c", "layout.c")
	if err := os.WriteFile(cPath, program(types, consts), 0o644); err != nil {
		log.Fatal(err)
	}
	tmp, err := os.MkdirTemp("", "layoutgen")
	if err != nil {
		log.Fatal(err)
	}
	defer os.RemoveAll(tmp)
	bin := filepath.Join(tmp, "layout")
	if out, err := exec.Command(*cc, "-std=c11", "-Wall", "-Werror", "-I", dir, "-o", bin, cPath).CombinedOutput(); err != nil {
		log.Fatalf("compiling %s: %v\n%s", cPath, err, out)
	}
	table, err := exec.Command(bin).Output()
	if err != nil {
		log.Fatalf("running the layout program: %v", err)
	}
	formatted, err := format.Source(table)
	if err != nil {
		log.Fatalf("the layout program printed invalid Go: %v", err)
	}
	if err := os.WriteFile(filepath.Join(*pkg, "layout_table_test.go"), formatted, 0o644); err != nil {
		log.Fatal(err)
	}
	log.Printf("%d structs, %d constants", len(types), len(consts))

	if *cross {
		asserts, err := exec.Command(bin, "assert").Output()
		if err != nil {
			log.Fatalf("running the layout program: %v", err)
		}
		assertPath := filepath.Join(tmp, "assert.c")
		if err := os.WriteFile(assertPath, asserts, 0o644); err != nil {
			log.Fatal(err)
		}
		for _, target := range crossTargets {
			out, err := exec.Command("clang", "--target="+target, "-std=c11", "-ffreestanding", "-fsyntax-only", "-I", dir, assertPath).CombinedOutput()
			if err != nil {
				log.Fatalf("the layouts differ on %s: %v\n%s", target, err, out)
			}
			log.Printf("same layouts on %s", target)
		}
	}
}

// crossTargets are the clang triples of Tela's GPU targets (ADR-281).
var crossTargets = []string{
	"x86_64-unknown-linux-gnu",
	"aarch64-unknown-linux-gnu",
	"x86_64-pc-windows-msvc",
	"aarch64-pc-windows-msvc",
}

// includeDir returns the first directory that holds the Vulkan headers.
func includeDir(flagValue string) (string, error) {
	candidates := []string{flagValue, os.Getenv("TELA_VULKAN_INCLUDE")}
	if home, err := os.UserHomeDir(); err == nil {
		candidates = append(candidates, filepath.Join(home, ".cache", "tela-vulkan-dev", "root", "usr", "include"))
	}
	candidates = append(candidates, "/usr/include")
	for _, dir := range candidates {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "vulkan", "vulkan_core.h")); err == nil {
			return dir, nil
		}
	}
	return "", fmt.Errorf("vulkan/vulkan_core.h not found; pass -include or set TELA_VULKAN_INCLUDE")
}

// goType is a type of structs.go and the C type it stands for.
type goType struct {
	name   string   // Go name; the C name is Vk + name
	fields []string // Go field names in order, without blank ones; nil when not a struct
	union  bool     // the C type is a union: only size and alignment compare
	cNames []string // members of the C struct, in order
}

// goTypes lists the types declared in structs.go.
func goTypes(path string) ([]goType, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []goType
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.TYPE {
			continue
		}
		for _, spec := range gen.Specs {
			ts := spec.(*ast.TypeSpec)
			t := goType{name: ts.Name.Name}
			if st, ok := ts.Type.(*ast.StructType); ok {
				t.fields = []string{}
				for _, f := range st.Fields.List {
					for _, n := range f.Names {
						if n.Name != "_" {
							t.fields = append(t.fields, n.Name)
						}
					}
				}
			}
			out = append(out, t)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%s declares no types", path)
	}
	return out, nil
}

var memberRE = regexp.MustCompile(`^.*?\b(\w+)(?:\[\w+\])*(?::\d+)?;$`)

// readC finds the C type in the header, takes its member names and checks
// that the Go struct has the same ones.
func (t *goType) readC(header string) error {
	cName := "Vk" + t.name
	for _, kind := range []string{"struct", "union"} {
		start := "typedef " + kind + " " + cName + " {\n"
		i := strings.Index(header, start)
		if i < 0 {
			continue
		}
		body := header[i+len(start):]
		end := strings.Index(body, "\n} "+cName+";")
		if end < 0 {
			return fmt.Errorf("%s: no end of the %s in the header", cName, kind)
		}
		t.union = kind == "union"
		for _, line := range strings.Split(body[:end], "\n") {
			if c := strings.Index(line, "//"); c >= 0 {
				line = line[:c]
			}
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			m := memberRE.FindStringSubmatch(line)
			if m == nil {
				return fmt.Errorf("%s: cannot read the member %q", cName, line)
			}
			t.cNames = append(t.cNames, m[1])
		}
		if t.union {
			if t.fields != nil {
				return fmt.Errorf("%s is a union: its Go type must not be a struct (only size and alignment are checked)", cName)
			}
			return nil
		}
		if t.fields == nil {
			return fmt.Errorf("%s is a struct and the Go type %s is not", cName, t.name)
		}
		var want []string
		for _, c := range t.cNames {
			want = append(want, goFieldName(c))
		}
		if strings.Join(want, " ") != strings.Join(t.fields, " ") {
			return fmt.Errorf("%s: the Go struct %s has the fields\n  %s\nand the header gives\n  %s", cName, t.name, strings.Join(t.fields, " "), strings.Join(want, " "))
		}
		return nil
	}
	return fmt.Errorf("the header has no struct or union %s (for the Go type %s)", cName, t.name)
}

// goFieldName is the Go name of a C member: the same with an upper-case
// first letter.
func goFieldName(c string) string {
	return strings.ToUpper(c[:1]) + c[1:]
}

// goConst is a constant of enums.go and the C constant of its trailing
// comment.
type goConst struct {
	name, cName string
	str         bool // a string constant
}

var cNameRE = regexp.MustCompile(`^VK_[A-Z0-9_a-z]+$`)

// goConsts lists the constants declared in enums.go. Each must name its
// C constant.
func goConsts(path string) ([]goConst, error) {
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, err
	}
	var out []goConst
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			vs := spec.(*ast.ValueSpec)
			cName := strings.TrimSpace(vs.Comment.Text())
			if len(vs.Names) != 1 || len(vs.Values) != 1 || !cNameRE.MatchString(cName) {
				return nil, fmt.Errorf("%s: the constant %s needs one value and a trailing comment that is its C name", path, vs.Names[0].Name)
			}
			lit, _ := vs.Values[0].(*ast.BasicLit)
			out = append(out, goConst{
				name:  vs.Names[0].Name,
				cName: cName,
				str:   lit != nil && lit.Kind == token.STRING,
			})
		}
	}
	return out, nil
}

// program returns the C program.
func program(types []goType, consts []goConst) []byte {
	var b bytes.Buffer
	b.WriteString(`// Code generated by internal/vk/internal/layoutgen; DO NOT EDIT.
//
// The names of the types, members and constants below are those of the
// Vulkan headers, Copyright 2015-2026 The Khronos Group Inc.,
// SPDX-License-Identifier: Apache-2.0 OR MIT.
//
// Prints the layout of the Vulkan structs and the values of the Vulkan
// constants that internal/vk binds, as the Go test file
// internal/vk/layout_table_test.go; with the argument "assert", prints
// the layouts as C static assertions instead, to compile for other
// targets. ` + "`go generate ./internal/vk`" + ` builds and runs it:
//
//	cc -std=c11 -I <dir of vulkan/vulkan_core.h> -o layout layout.c
//	./layout | gofmt > ../../../layout_table_test.go
//
// No Go command compiles this file: its directory has no Go files.

#include <stddef.h>
#include <stdio.h>
#include <string.h>

#include <vulkan/vulkan_core.h>

// What layouts prints.
enum { TABLE, GO_ASSERTS, C_ASSERTS };
static int mode;

#define SIZE(T, G, isUnion, open) \
	switch (mode) { \
	case TABLE: \
		printf("\t{\"" #T "\", reflect.TypeOf(" #G "{}), %zu, %zu, " isUnion ", " open "\n", sizeof(T), _Alignof(T)); \
		break; \
	case GO_ASSERTS: \
		printf("\t_ = [1]struct{}{}[unsafe.Sizeof(" #G "{})-%zu]\n\t_ = [1]struct{}{}[unsafe.Alignof(" #G "{})-%zu]\n", sizeof(T), _Alignof(T)); \
		break; \
	case C_ASSERTS: \
		printf("_Static_assert(sizeof(" #T ") == %zu && _Alignof(" #T ") == %zu, \"" #T "\");\n", sizeof(T), _Alignof(T)); \
		break; \
	}
#define STRUCT(T, G) SIZE(T, G, "false", "[]fieldLayout{")
#define UNION(T, G) SIZE(T, G, "true", "nil},")
#define FIELD(T, G, f, F) \
	switch (mode) { \
	case TABLE: \
		printf("\t\t{\"" #f "\", unsafe.Offsetof(" #G "{}." #F "), unsafe.Sizeof(" #G "{}." #F "), %zu, %zu},\n", offsetof(T, f), sizeof(((T *)0)->f)); \
		break; \
	case GO_ASSERTS: \
		printf("\t_ = [1]struct{}{}[unsafe.Offsetof(" #G "{}." #F ")-%zu]\n\t_ = [1]struct{}{}[unsafe.Sizeof(" #G "{}." #F ")-%zu]\n", offsetof(T, f), sizeof(((T *)0)->f)); \
		break; \
	case C_ASSERTS: \
		printf("_Static_assert(offsetof(" #T ", " #f ") == %zu && sizeof(((" #T " *)0)->" #f ") == %zu, \"" #T "." #f "\");\n", offsetof(T, f), sizeof(((T *)0)->f)); \
		break; \
	}
#define END() \
	if (mode == TABLE) \
		printf("\t}},\n")
#define CONST(c, G) \
	printf("\t{\"" #c "\", u64(" #G "), %#llx},\n", (unsigned long long)(long long)(c))
#define STRCONST(c, G) \
	printf("\t{\"" #c "\", " #G ", \"%s\"},\n", c)

// layouts prints every struct and union, in the form of mode.
static void layouts(void) {
`)
	for _, t := range types {
		if t.union {
			fmt.Fprintf(&b, "\tUNION(Vk%s, %s);\n", t.name, t.name)
			continue
		}
		fmt.Fprintf(&b, "\tSTRUCT(Vk%s, %s);\n", t.name, t.name)
		for _, c := range t.cNames {
			fmt.Fprintf(&b, "\tFIELD(Vk%s, %s, %s, %s);\n", t.name, t.name, c, goFieldName(c))
		}
		b.WriteString("\tEND();\n")
	}
	b.WriteString(`}

int main(int argc, char **argv) {
	if (argc > 1 && strcmp(argv[1], "assert") == 0) {
		mode = C_ASSERTS;
		printf("#include <stddef.h>\n#include <vulkan/vulkan_core.h>\n\n");
		layouts();
		return 0;
	}

	printf("// Code generated by internal/vk/internal/layoutgen from the Vulkan headers %d.%d.%d; DO NOT EDIT.\n\n",
		VK_API_VERSION_MAJOR(VK_HEADER_VERSION_COMPLETE), VK_API_VERSION_MINOR(VK_HEADER_VERSION_COMPLETE), VK_API_VERSION_PATCH(VK_HEADER_VERSION_COMPLETE));
	printf("//go:build telagpu && (linux || windows)\n\npackage vk\n\nimport (\n\t\"reflect\"\n\t\"unsafe\"\n)\n\n");

	printf("// layoutTable is the layout of the C structs and unions that structs.go\n// binds, beside the layout of their Go types.\n");
	printf("var layoutTable = []structLayout{\n");
	mode = TABLE;
	layouts();
	printf("}\n\n");

	printf("// The same layouts as assertions the compiler checks: an index is out of\n// range (a constant overflows) where a size, an alignment or an offset\n// differs. They make cross-compiling the tests check the Go layouts of the\n// targets whose tests do not run.\n");
	printf("var (\n");
	mode = GO_ASSERTS;
	layouts();
	printf(")\n\n");

	printf("// constTable is the value of the C constants that enums.go binds, beside\n// the Go ones.\n");
	printf("var constTable = []constValue{\n");

`)
	for _, c := range consts {
		if !c.str {
			fmt.Fprintf(&b, "\tCONST(%s, %s);\n", c.cName, c.name)
		}
	}
	b.WriteString(`
	printf("}\n\n");
	printf("// stringConstTable is the same for the string constants.\n");
	printf("var stringConstTable = []stringConst{\n");

`)
	for _, c := range consts {
		if c.str {
			fmt.Fprintf(&b, "\tSTRCONST(%s, %s);\n", c.cName, c.name)
		}
	}
	b.WriteString(`
	printf("}\n");
	return 0;
}
`)
	return b.Bytes()
}
