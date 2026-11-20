package core

import (
	"runtime"
	"strings"
)

// Creation sites (ADR-299): where the user's code made a node, for
// ElementInfo.Site (tela devtools' "source line", the [src=...] of aria
// snapshots).
//
// While an instrumented tree in dev mode exists (insSites), the node
// constructors (Primitive, Text, TextFn, C, CP, Memo, Provider, Presence,
// Portal, VirtualList) and the conversion of a struct component into its
// node (componentOf) capture the return addresses above them into the
// node's desc.site, the field a dynamic list's key warnings use (the
// list's own capture is kept). Nothing is resolved then: an element's site
// is resolved when it is listed, from the node of its last build or
// update, and cached per stack. Off, the constructors pay one atomic load.
//
// The site is a frame of the captured stack, read from the constructor's
// caller outwards and no further than the build machinery (a method of
// Tree, or the component function's caller):
//
//  1. the first frame of the app: outside Tela's packages (the module
//     github.com/kuronosu/tela but its examples), or in a _test.go file,
//     as the hook-boundary check counts them (ADR-085), so Div(...) in
//     settings.go, and Settings{...} given to an element there, are
//     settings.go:42;
//  2. without one, the first frame outside Tela's plumbing (package tela,
//     h and internal/...): the node was made by a library component's own
//     code, such as the elements kit.Button builds, which are kit's
//     button.go line (also when the app itself is in kit, like
//     examples/kit, whose kit.Button values are at its own lines by rule
//     1);
//  3. a struct component returned by a build (return Card{...} from a
//     Build or a function component) is converted after that build
//     returned, with no frame of it on the stack: its site is the
//     declaration of the component that returned it (the line of func
//     (p Page) Build).
//
// A node made outside dev mode, or before the instrumentation turned on,
// has no site until its parent builds again.

// maxSiteCache bounds the resolved sites cached per tree (cleared when
// full).
const maxSiteCache = 4096

// noteSite records the call stack above the node constructor that calls
// it in d.site (skip 0), while creation sites are captured.
//
//go:noinline
func noteSite(d *desc) {
	if d.site != nil {
		return
	}
	s := new(callStack)
	runtime.Callers(3, s[:]) // runtime.Callers, noteSite, the constructor
	d.site = s
}

// declSite returns a stack whose one frame is the declaration of the
// function at code pointer pc (a component's function or Build method):
// the site of a struct component a build returned (rule 3). A return
// address is looked up one byte before it, so pc+1 resolves to the
// function's entry, whose line is the func keyword's.
func declSite(pc uintptr) *callStack {
	if pc == 0 {
		return nil
	}
	return &callStack{pc + 1}
}

// creationSite resolves a captured stack with the rules above.
func (ins *instrument) creationSite(s *callStack) SourceSite {
	if site, ok := ins.sites[*s]; ok {
		return site
	}
	site := resolveCreation(s)
	if ins.sites == nil || len(ins.sites) >= maxSiteCache {
		ins.sites = map[callStack]SourceSite{}
	}
	ins.sites[*s] = site
	return site
}

func resolveCreation(s *callStack) SourceSite {
	n := 0
	for n < len(s) && s[n] != 0 {
		n++
	}
	if n == 0 {
		return SourceSite{}
	}
	var lib runtime.Frame
	frames := runtime.CallersFrames(s[:n])
	for {
		f, more := frames.Next()
		if f.Function != "" {
			if buildMachinery(f.Function) {
				break
			}
			if appFrame(f.Function, f.File) {
				return frameSite(f)
			}
			if lib.Function == "" && !frameworkFrame(f.Function, f.File) {
				lib = f
			}
		}
		if !more {
			break
		}
	}
	if lib.Function != "" {
		return frameSite(lib)
	}
	return SourceSite{}
}

func frameSite(f runtime.Frame) SourceSite {
	return SourceSite{Func: goName(f.Function), File: f.File, Line: f.Line}
}

// buildMachinery reports a frame of core that runs component functions:
// the frames past it are not where the node was made.
func buildMachinery(fn string) bool {
	rest, ok := strings.CutPrefix(fn, corePkg+".")
	return ok && (strings.HasPrefix(rest, "(*Tree).") || strings.HasPrefix(rest, "buildFrame") ||
		strings.HasPrefix(rest, "(*compDesc).run"))
}

// appFrame reports a frame of the app: not in a Tela package (the module's
// examples are apps), or in a _test.go file.
func appFrame(fn, file string) bool {
	if strings.HasSuffix(file, "_test.go") {
		return true
	}
	rest, ok := strings.CutPrefix(fn, telaPkg)
	if !ok || rest == "" || rest[0] != '.' && rest[0] != '/' {
		return true
	}
	return strings.HasPrefix(rest, "/examples/")
}
