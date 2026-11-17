package main

import (
	"fmt"
	"go/ast"
	"go/constant"
	"go/token"
	"strconv"

	"github.com/kuronosu/tela"
)

// The declaration of the app (ADR-294) read from the tela.App literal, as
// appinfo.go reads AppID and Title: without running the program, and an
// error that names the field and the way out for what is not constant.

// declarationFields are the fields of appSource.Errs in the order of
// tela.App.
var declarationFields = []string{"Publisher", "Description", "Category", "Schemes", "FileTypes", "Services"}

// DeclarationErr returns the error of the first unreadable field of the
// declaration, or nil.
func (a appSource) DeclarationErr() error {
	for _, f := range declarationFields {
		if err := a.Errs[f]; err != nil {
			return err
		}
	}
	return nil
}

// maxDeclared bounds the entries of the declaration's lists.
const maxDeclared = 256

// constString returns the constant string value of e.
func (s *appScan) constString(e ast.Expr) (string, bool) {
	if tv, ok := s.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.String {
		return constant.StringVal(tv.Value), true
	}
	if bl, ok := e.(*ast.BasicLit); ok && bl.Kind == token.STRING {
		if v, err := strconv.Unquote(bl.Value); err == nil {
			return v, true
		}
	}
	return "", false
}

// constBool returns the constant boolean value of e.
func (s *appScan) constBool(e ast.Expr) (bool, bool) {
	if tv, ok := s.info.Types[e]; ok && tv.Value != nil && tv.Value.Kind() == constant.Bool {
		return constant.BoolVal(tv.Value), true
	}
	if id, ok := e.(*ast.Ident); ok && (id.Name == "true" || id.Name == "false") {
		return id.Name == "true", true
	}
	return false, false
}

// fieldExpr returns the value of the field name of lit, and whether it is
// there.
func fieldExpr(lit *ast.CompositeLit, name string) (ast.Expr, bool) {
	for _, el := range lit.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			continue
		}
		if k, ok := kv.Key.(*ast.Ident); ok && k.Name == name {
			return kv.Value, true
		}
	}
	return nil, false
}

// readDeclaration reads the fields of ADR-294 of lit into out.
func (s *appScan) readDeclaration(lit *ast.CompositeLit, out *appSource) {
	fail := func(name string, err error) {
		if out.Errs == nil {
			out.Errs = map[string]error{}
		}
		out.Errs[name] = err
	}
	for _, f := range []struct {
		name string
		dst  *string
	}{{"Publisher", &out.Publisher}, {"Description", &out.Description}, {"Category", &out.Category}} {
		v, err := s.stringField(lit, f.name)
		if err != nil {
			fail(f.name, err)
			continue
		}
		*f.dst = v
	}
	if e, ok := fieldExpr(lit, "Schemes"); ok {
		v, err := s.stringList(e)
		if err != nil {
			fail("Schemes", fmt.Errorf("the Schemes of the tela.App %v", err))
		}
		out.Schemes = v
	}
	if e, ok := fieldExpr(lit, "FileTypes"); ok {
		v, err := s.fileTypes(e)
		if err != nil {
			fail("FileTypes", fmt.Errorf("the FileTypes of the tela.App %v", err))
		}
		out.FileTypes = v
	}
	if e, ok := fieldExpr(lit, "Services"); ok {
		v, err := s.services(e)
		if err != nil {
			fail("Services", fmt.Errorf("the Services of the tela.App %v", err))
		} else {
			out.Services = v
		}
	}
}

// follow returns the expression that the variable e stands for (a package
// level one or one of main), looking through parentheses, or e itself.
func (s *appScan) follow(e ast.Expr) ast.Expr {
	for range maxResolve {
		switch x := e.(type) {
		case *ast.ParenExpr:
			e = x.X
			continue
		case *ast.Ident:
			if v, ok := s.locals[x.Name]; ok {
				e = v
				continue
			}
			if v, ok := s.vars[x.Name]; ok {
				e = v
				continue
			}
		}
		break
	}
	return e
}

// isNil reports whether e is the predeclared nil.
func isNil(e ast.Expr) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == "nil"
}

// listLit returns the elements of the slice literal that e is or stands for
// (none for nil), and an error saying what is expected otherwise.
func (s *appScan) listLit(e ast.Expr, what string) ([]ast.Expr, error) {
	e = s.follow(e)
	if isNil(e) {
		return nil, nil
	}
	cl, ok := e.(*ast.CompositeLit)
	if !ok {
		return nil, fmt.Errorf("is not %s that tela can read without running the program; write it as a slice literal, or a package-level variable that holds one", what)
	}
	if len(cl.Elts) > maxDeclared {
		return nil, fmt.Errorf("has %d entries, more than the %d that tela reads", len(cl.Elts), maxDeclared)
	}
	return cl.Elts, nil
}

// stringList reads a slice literal of constant strings.
func (s *appScan) stringList(e ast.Expr) ([]string, error) {
	elts, err := s.listLit(e, "a slice literal of constant strings")
	if err != nil {
		return nil, err
	}
	var out []string
	for i, el := range elts {
		v, ok := s.constString(el)
		if !ok {
			return nil, fmt.Errorf("has an entry (%d) that is not a constant string that tela can read without running the program (a literal, a constant or a concatenation of them)", i)
		}
		out = append(out, v)
	}
	return out, nil
}

// roleNames are the roles of tela.HandlerRole by their names.
var roleNames = map[string]tela.HandlerRole{
	"HandlerDefault": tela.HandlerDefault,
	"HandlerEditor":  tela.HandlerEditor,
	"HandlerViewer":  tela.HandlerViewer,
	"HandlerShell":   tela.HandlerShell,
	"HandlerNone":    tela.HandlerNone,
}

// telaName returns the name e has in the tela package: Name for tela.Name,
// and for Name with a dot import.
func (s *appScan) telaName(e ast.Expr) (string, bool) {
	switch e := e.(type) {
	case *ast.ParenExpr:
		return s.telaName(e.X)
	case *ast.SelectorExpr:
		if id, ok := e.X.(*ast.Ident); ok && s.tela[id.Name] {
			return e.Sel.Name, true
		}
	case *ast.Ident:
		if s.dot {
			return e.Name, true
		}
	}
	return "", false
}

// role reads tela.HandlerViewer and its siblings.
func (s *appScan) role(e ast.Expr) (tela.HandlerRole, bool) {
	name, ok := s.telaName(e)
	if !ok {
		return 0, false
	}
	r, ok := roleNames[name]
	return r, ok
}

// fileTypes reads a slice literal of tela.FileType literals.
func (s *appScan) fileTypes(e ast.Expr) ([]tela.FileType, error) {
	elts, err := s.listLit(e, "a slice literal of tela.FileType")
	if err != nil {
		return nil, err
	}
	var out []tela.FileType
	for i, el := range elts {
		el = s.follow(el)
		cl, ok := el.(*ast.CompositeLit)
		if ok && cl.Type != nil {
			name, isTela := s.telaName(cl.Type)
			ok = isTela && name == "FileType"
		}
		if !ok {
			return nil, fmt.Errorf("has an entry (%d) that is not a tela.FileType literal; write it as {Ext: \".note\", MIME: \"application/x-notes-note\", Description: \"Notes document\"}", i)
		}
		ft, err := s.fileType(cl, i)
		if err != nil {
			return nil, err
		}
		out = append(out, ft)
	}
	return out, nil
}

// fileType reads the fields of one tela.FileType literal, the entry i.
func (s *appScan) fileType(cl *ast.CompositeLit, i int) (tela.FileType, error) {
	var ft tela.FileType
	str := func(field string, dst *string) func(ast.Expr) error {
		return func(v ast.Expr) error {
			x, ok := s.constString(v)
			if !ok {
				return fmt.Errorf("has an entry (%d) whose %s is not a constant string that tela can read without running the program (a literal, a constant or a concatenation of them)", i, field)
			}
			*dst = x
			return nil
		}
	}
	fields := []struct {
		name string
		set  func(ast.Expr) error
	}{
		{"Ext", str("Ext", &ft.Ext)},
		{"MIME", str("MIME", &ft.MIME)},
		{"Description", str("Description", &ft.Description)},
		{"Role", func(v ast.Expr) error {
			r, ok := s.role(v)
			if !ok {
				return fmt.Errorf("has an entry (%d) whose Role is not one of tela.HandlerEditor, tela.HandlerViewer, tela.HandlerShell, tela.HandlerNone or tela.HandlerDefault; tela reads the names, not other expressions", i)
			}
			ft.Role = r
			return nil
		}},
	}
	for j, f := range cl.Elts {
		idx := j // positional: the order of the fields
		if kv, ok := f.(*ast.KeyValueExpr); ok {
			k, _ := kv.Key.(*ast.Ident)
			idx = -1
			for n, fd := range fields {
				if k != nil && k.Name == fd.name {
					idx = n
				}
			}
			if idx < 0 {
				return ft, fmt.Errorf("has an entry (%d) with a field tela.FileType does not have; it has Ext, MIME, Description and Role", i)
			}
			f = kv.Value
		}
		if idx >= len(fields) {
			return ft, fmt.Errorf("has an entry (%d) with more than four values; tela.FileType has Ext, MIME, Description and Role", i)
		}
		if err := fields[idx].set(f); err != nil {
			return ft, err
		}
	}
	return ft, nil
}

// services reads a tela.Services literal.
func (s *appScan) services(e ast.Expr) (tela.Services, error) {
	var out tela.Services
	e = s.follow(e)
	if u, ok := e.(*ast.UnaryExpr); ok {
		e = s.follow(u.X)
	}
	cl, ok := e.(*ast.CompositeLit)
	if ok {
		name, isTela := s.telaName(cl.Type)
		ok = cl.Type != nil && isTela && name == "Services"
	}
	if !ok {
		return out, fmt.Errorf("is not a tela.Services literal that tela can read without running the program; write it as tela.Services{Camera: \"...\", Network: true}, or a package-level variable that holds one")
	}
	strs := map[string]*string{
		"Camera": &out.Camera, "Microphone": &out.Microphone, "Location": &out.Location,
		"Contacts": &out.Contacts, "Calendars": &out.Calendars, "Photos": &out.Photos, "Bluetooth": &out.Bluetooth,
	}
	bools := map[string]*bool{
		"Network": &out.Network, "Notifications": &out.Notifications, "Secrets": &out.Secrets,
		"GlobalShortcuts": &out.GlobalShortcuts, "Files": &out.Files,
	}
	for _, el := range cl.Elts {
		kv, ok := el.(*ast.KeyValueExpr)
		if !ok {
			return out, fmt.Errorf("has a value without its field name; write each one as Name: value")
		}
		k, _ := kv.Key.(*ast.Ident)
		if k == nil {
			return out, fmt.Errorf("has a key that is not a field name")
		}
		if p, ok := strs[k.Name]; ok {
			v, ok := s.constString(kv.Value)
			if !ok {
				return out, fmt.Errorf("has a %s usage string that is not a constant string that tela can read without running the program (a literal, a constant or a concatenation of them)", k.Name)
			}
			*p = v
		} else if p, ok := bools[k.Name]; ok {
			v, ok := s.constBool(kv.Value)
			if !ok {
				return out, fmt.Errorf("has a %s that is not true or false written in the source, which tela can read without running the program", k.Name)
			}
			*p = v
		} else {
			return out, fmt.Errorf("sets %s, which tela.Services does not have", k.Name)
		}
	}
	return out, nil
}
