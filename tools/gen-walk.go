//go:build ignore

// Command gen-walk writes walk.go: a depth-first traversal over every node kind
// the parser can produce.
//
// Hand-writing this is the kind of job nobody finishes. There are 77 node kinds,
// and the children of a node are not reached uniformly -- SBlock holds []Stmt,
// SLocal holds []Decl which holds a Binding and an Expr, STry holds *Catch and
// an SBlock by value, EArrow holds []Arg and an FnBody. Miss one field and the
// walker silently skips a subtree, which is the worst failure mode a traversal
// can have: quiet, correct-looking, and wrong only for the inputs you did not
// test. Upstream adds node kinds and fields on roughly a monthly cadence, so a
// hand-written walker starts drifting immediately.
//
// So it is derived from the same source of truth as the seam. A field is walked
// when its type can carry a Stmt, Expr or Binding -- directly, through a slice
// or pointer, or through an intermediate struct like Decl or Property. That
// closure is computed, not listed, so a new field on an existing node is picked
// up by regenerating.
//
// Analysis is confined to js_ast. Nothing outside it can hold a node: ast and
// logger sit BELOW js_ast in the import graph, so they cannot name its types.
package main

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The three node interfaces, by the marker method their implementations carry.
var markers = []struct{ method, wrapper, walker string }{
	{"isStmt", "Stmt", "walkStmt"},
	{"isExpr", "Expr", "walkExpr"},
	{"isBinding", "Binding", "walkBinding"},
}

type structType struct {
	name   string
	fields []field
}

type field struct {
	name string
	typ  ast.Expr
}

var (
	structs = map[string]*structType{} // name -> decl, js_ast only
	impls   = map[string][]string{}    // marker method -> concrete type names
	carries = map[string]int{}         // name -> 0 unknown, 1 in progress, 2 yes, 3 no
	helpers = map[string]bool{}        // struct types needing a walkX helper
)

func main() {
	root, err := os.Getwd()
	check(err)
	dir := filepath.Join(root, "internal", "js_ast")

	files, err := os.ReadDir(dir)
	check(err)
	for _, fi := range files {
		if !strings.HasSuffix(fi.Name(), ".go") || strings.HasSuffix(fi.Name(), "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, filepath.Join(dir, fi.Name()), nil, 0)
		check(err)
		for _, d := range f.Decls {
			switch d := d.(type) {
			case *ast.FuncDecl:
				collectMarker(d)
			case *ast.GenDecl:
				for _, s := range d.Specs {
					ts, ok := s.(*ast.TypeSpec)
					if !ok || !ts.Name.IsExported() {
						continue
					}
					st, ok := ts.Type.(*ast.StructType)
					if !ok {
						continue
					}
					sd := &structType{name: ts.Name.Name}
					for _, fl := range st.Fields.List {
						for _, n := range fl.Names {
							if n.IsExported() {
								sd.fields = append(sd.fields, field{n.Name, fl.Type})
							}
						}
					}
					structs[ts.Name.Name] = sd
				}
			}
		}
	}

	for _, m := range markers {
		sort.Strings(impls[m.method])
		if len(impls[m.method]) == 0 {
			fmt.Fprintf(os.Stderr, "gen-walk: no implementations found for %s()\n", m.method)
			os.Exit(1)
		}
	}

	// Emit the body first: walking it is what discovers which intermediate
	// structs need helpers, and helpers can discover further helpers.
	body := &bytes.Buffer{}
	for _, m := range markers {
		renderWalker(body, m.method, m.wrapper, m.walker)
	}
	renderHelpers(body)

	out := &bytes.Buffer{}
	renderPrologue(out)
	out.Write(body.Bytes())
	writeFile(filepath.Join(root, "walk.go"), out.Bytes())
}

func collectMarker(d *ast.FuncDecl) {
	if d.Recv == nil || len(d.Recv.List) != 1 {
		return
	}
	for _, m := range markers {
		if d.Name.Name != m.method {
			continue
		}
		star, ok := d.Recv.List[0].Type.(*ast.StarExpr)
		if !ok {
			return
		}
		if id, ok := star.X.(*ast.Ident); ok {
			impls[m.method] = append(impls[m.method], id.Name)
		}
	}
}

// typeCarries reports whether a type expression can reach a node wrapper.
func typeCarries(e ast.Expr) bool {
	switch t := e.(type) {
	case *ast.StarExpr:
		return typeCarries(t.X)
	case *ast.ArrayType:
		return typeCarries(t.Elt)
	case *ast.Ident:
		return nameCarries(t.Name)
	}
	// Selector (ast.X, logger.X), map, func, interface: cannot hold a node.
	return false
}

// nameCarries walks the struct graph, memoised. A cycle in progress answers
// "no" for the cycle edge itself, which is correct: if a type reaches a node it
// does so through some other field, and that field settles the answer.
func nameCarries(name string) bool {
	switch name {
	case "Stmt", "Expr", "Binding":
		return true
	}
	switch carries[name] {
	case 1, 3:
		return false
	case 2:
		return true
	}
	sd := structs[name]
	if sd == nil {
		carries[name] = 3
		return false
	}
	carries[name] = 1
	result := false
	for _, f := range sd.fields {
		if typeCarries(f.typ) {
			result = true
			break
		}
	}
	if result {
		carries[name] = 2
	} else {
		carries[name] = 3
	}
	return result
}

// emitField writes the traversal for one value of a given type. expr is the Go
// expression naming it, already addressable.
func emitField(buf *bytes.Buffer, expr string, e ast.Expr, depth int) {
	switch t := e.(type) {
	case *ast.StarExpr:
		fmt.Fprintf(buf, "if %s != nil {\n", expr)
		emitField(buf, "("+deref(expr)+")", t.X, depth+1)
		buf.WriteString("}\n")
	case *ast.ArrayType:
		idx := fmt.Sprintf("i%d", depth)
		fmt.Fprintf(buf, "for %s := range %s {\n", idx, expr)
		emitField(buf, fmt.Sprintf("%s[%s]", expr, idx), t.Elt, depth+1)
		buf.WriteString("}\n")
	case *ast.Ident:
		switch t.Name {
		case "Stmt":
			fmt.Fprintf(buf, "walkStmt(&%s, f)\n", expr)
		case "Expr":
			fmt.Fprintf(buf, "walkExpr(&%s, f)\n", expr)
		case "Binding":
			fmt.Fprintf(buf, "walkBinding(&%s, f)\n", expr)
		default:
			if nameCarries(t.Name) {
				helpers[t.Name] = true
				fmt.Fprintf(buf, "walk%s(&%s, f)\n", t.Name, expr)
			}
		}
	}
}

// deref strips one level of pointer for a StarExpr child, keeping the emitted
// expression addressable.
func deref(expr string) string { return "*" + expr }

// emitStructBody writes the traversal of every child-bearing field of a struct,
// with recv naming a pointer to it.
func emitStructBody(buf *bytes.Buffer, name, recv string) {
	sd := structs[name]
	if sd == nil {
		return
	}
	for _, f := range sd.fields {
		if typeCarries(f.typ) {
			emitField(buf, recv+"."+f.name, f.typ, 0)
		}
	}
}

func renderWalker(buf *bytes.Buffer, method, wrapper, walker string) {
	lower := strings.ToLower(wrapper[:1]) + wrapper[1:]
	fmt.Fprintf(buf, `
func %s(%s *%s, f func(Node) bool) {
	if %s.Data == nil || !f(Node{Loc: %s.Loc, Data: %s.Data}) {
		return
	}
	switch n := %s.Data.(type) {
`, walker, lower, wrapper, lower, lower, lower, lower)

	for _, name := range impls[method] {
		inner := &bytes.Buffer{}
		emitStructBody(inner, name, "n")
		if inner.Len() == 0 {
			continue // a leaf: ENull, SEmpty, BMissing, ...
		}
		fmt.Fprintf(buf, "case *%s:\n", name)
		buf.Write(inner.Bytes())
	}
	buf.WriteString("}\n}\n")
}

func renderHelpers(buf *bytes.Buffer) {
	// emitStructBody can add helpers while we are emitting them, so drain
	// until the set stops growing.
	done := map[string]bool{}
	for {
		var pending []string
		for name := range helpers {
			if !done[name] {
				pending = append(pending, name)
			}
		}
		if len(pending) == 0 {
			return
		}
		sort.Strings(pending)
		for _, name := range pending {
			done[name] = true
			inner := &bytes.Buffer{}
			emitStructBody(inner, name, "v")
			fmt.Fprintf(buf, "\nfunc walk%s(v *%s, f func(Node) bool) {\n", name, name)
			buf.Write(inner.Bytes())
			buf.WriteString("}\n")
		}
	}
}

func renderPrologue(buf *bytes.Buffer) {
	buf.WriteString(`// Code generated by tools/gen-walk.go; DO NOT EDIT.

package jsast

// Node is one visited node: the statement, expression or binding data, paired
// with the Loc of the wrapper it was reached through.
//
// Data holds a node kind pointer -- *SExpr, *EDot, *BIdentifier -- so a visitor
// type-switches on it exactly as it would on Stmt.Data.
type Node struct {
	Loc  Loc
	Data any
}

// Inspect traverses stmts in depth-first order, calling f for every statement,
// expression and binding it reaches. If f returns false, that node's children
// are skipped; returning true descends.
//
// A node is visited before its children. Fields following the OrNil convention
// are skipped when empty, so f never sees a Node with nil Data.
func Inspect(stmts []Stmt, f func(Node) bool) {
	for i := range stmts {
		walkStmt(&stmts[i], f)
	}
}

// InspectExpr is Inspect rooted at a single expression.
func InspectExpr(e Expr, f func(Node) bool) { walkExpr(&e, f) }
`)
}

func writeFile(path string, src []byte) {
	out, err := format.Source(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-walk: %s: %v\n%s\n", path, err, src)
		os.Exit(1)
	}
	check(os.WriteFile(path, out, 0o644))
	fmt.Fprintf(os.Stderr, "gen-walk: wrote %s\n", filepath.Base(path))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-walk:", err)
		os.Exit(1)
	}
}
