//go:build ignore

// Command gen-seam writes export.go and constants.go from the vendored tree.
//
// The seam used to be a hand-copied list, which is a bad shape for a file whose
// contents are a pure function of internal/: an upstream release that ADDS a
// type or a constant does not break the build, so a stale list stays green and
// silently withholds the new surface from consumers. That already happened --
// ast's enum types were aliased while their values were not, leaving
// ImportRecord.Kind readable and jsast.ImportStmt undefined.
//
// So the surface is derived instead of listed. Two rules:
//
//	types: every exported type of js_ast, plus anything reachable from one
//	       through an exported field, transitively, within the seam packages
//	consts: every exported constant whose declared type is in that set
//
// The const rule is what fixes the bug: a value can be named exactly when the
// thing that holds it can be named. It also drops js_ast.NSExportPartIndex on
// its own -- a bundler part index typed uint32, not an AST enum -- without an
// exclusion list saying so.
//
// Analysis is syntactic. That is enough here (the input is one flat package per
// directory with no build-tag variance in the type declarations) and keeps the
// generator free of dependencies.
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

const modulePath = "github.com/bytevet/esbuild-jsast"

// seam packages, in emission order. The local name is what the generated files
// import them as; esast avoids colliding with go/ast in the generator's own
// head and reads better next to js_ast at the call site.
var pkgs = []struct{ dir, local string }{
	{"js_ast", "js_ast"},
	{"ast", "esast"},
	{"logger", "logger"},
}

// Seeds. Everything exported by js_ast is AST by definition; the other packages
// contribute only what the AST actually reaches, which is why logger lands at
// Loc/Range/Path rather than its 29 types of diagnostic plumbing.
const seedPkg = "js_ast"

// Renamer and minifier state hang off no AST node, but Symbol and ImportRecord
// sit next to them in the same file. Reachability already excludes them; this
// list exists so that a future upstream change wiring one INTO a node is a
// deliberate decision here rather than a silent surface expansion.
var neverExport = map[string]bool{
	"ast.CharFreq":      true,
	"ast.NameMinifier":  true,
	"ast.SlotCounts":    true,
	"ast.SlotNamespace": true,
}

// Extra seeds: exported before the seam was derived, and no AST field points at
// them, so reachability alone would drop them. Removing a name from a published
// API is a break whatever the reason, so they are pinned here instead.
var alwaysExport = []string{
	"ast.SymbolMap", // a consumer assembling symbol tables across files holds one
}

type namedType struct {
	pkg, name string
	fields    []ast.Expr // exported field types, for reachability
}

type constDecl struct {
	pkg, name, typ string
	order          int
}

func main() {
	root, err := os.Getwd()
	check(err)

	types := map[string]*namedType{} // "pkg.Name" -> decl
	var consts []constDecl           // in declaration order
	imports := map[string]string{}   // "pkg:localalias" -> real pkg dir

	for _, p := range pkgs {
		dir := filepath.Join(root, "internal", p.dir)
		files, err := os.ReadDir(dir)
		check(err)
		order := 0
		for _, fi := range files {
			name := fi.Name()
			if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
				continue
			}
			fset := token.NewFileSet()
			f, err := parser.ParseFile(fset, filepath.Join(dir, name), nil, 0)
			check(err)

			// Record this file's import aliases so a field written as
			// "logger.Loc" can be resolved to the logger package.
			for _, im := range f.Imports {
				path := strings.Trim(im.Path.Value, `"`)
				if !strings.HasPrefix(path, modulePath+"/internal/") {
					continue
				}
				target := strings.TrimPrefix(path, modulePath+"/internal/")
				alias := target
				if im.Name != nil {
					alias = im.Name.Name
				}
				imports[p.dir+":"+alias] = target
			}

			for _, d := range f.Decls {
				gd, ok := d.(*ast.GenDecl)
				if !ok {
					continue
				}
				var lastType ast.Expr // const groups carry the type down an iota run
				for _, s := range gd.Specs {
					switch s := s.(type) {
					case *ast.TypeSpec:
						if !s.Name.IsExported() {
							continue
						}
						nt := &namedType{pkg: p.dir, name: s.Name.Name}
						if st, ok := s.Type.(*ast.StructType); ok {
							for _, fld := range st.Fields.List {
								if exportedField(fld) {
									nt.fields = append(nt.fields, fld.Type)
								}
							}
						}
						types[p.dir+"."+s.Name.Name] = nt
					case *ast.ValueSpec:
						if gd.Tok != token.CONST {
							continue
						}
						if s.Type != nil {
							lastType = s.Type
						}
						for _, n := range s.Names {
							if !n.IsExported() {
								continue
							}
							consts = append(consts, constDecl{
								pkg:   p.dir,
								name:  n.Name,
								typ:   qualify(p.dir, lastType, imports),
								order: order,
							})
							order++
						}
					}
				}
			}
		}
	}

	// Reachability: seed with every js_ast type, then follow exported fields.
	keep := map[string]bool{}
	var queue []string
	for key, t := range types {
		if t.pkg == seedPkg && !neverExport[key] {
			keep[key] = true
			queue = append(queue, key)
		}
	}
	for _, key := range alwaysExport {
		if types[key] == nil {
			fmt.Fprintf(os.Stderr, "gen-seam: alwaysExport names %s, which upstream no longer defines\n", key)
			os.Exit(1)
		}
		if !keep[key] {
			keep[key] = true
			queue = append(queue, key)
		}
	}
	sort.Strings(queue)
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		t := types[cur]
		if t == nil {
			continue
		}
		for _, fieldType := range t.fields {
			for _, ref := range referenced(t.pkg, fieldType, imports) {
				if keep[ref] || neverExport[ref] || types[ref] == nil {
					continue
				}
				keep[ref] = true
				queue = append(queue, ref)
			}
		}
	}

	writeFile(filepath.Join(root, "export.go"), renderTypes(keep))
	writeFile(filepath.Join(root, "constants.go"), renderConsts(keep, consts))
}

// exportedField reports whether a struct field is reachable by a consumer.
// Embedded fields are exported when their type name is.
func exportedField(f *ast.Field) bool {
	if len(f.Names) == 0 {
		for _, r := range referencedNames(f.Type) {
			return ast.IsExported(r)
		}
		return false
	}
	for _, n := range f.Names {
		if n.IsExported() {
			return true
		}
	}
	return false
}

// referenced returns the "pkg.Name" keys a field type mentions, unwrapping
// pointers, slices, arrays and maps to find the named types underneath.
func referenced(fromPkg string, e ast.Expr, imports map[string]string) []string {
	var out []string
	ast.Inspect(e, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.SelectorExpr:
			if id, ok := n.X.(*ast.Ident); ok {
				if target, ok := imports[fromPkg+":"+id.Name]; ok {
					out = append(out, target+"."+n.Sel.Name)
				}
			}
			return false
		case *ast.Ident:
			if n.IsExported() {
				out = append(out, fromPkg+"."+n.Name)
			}
		}
		return true
	})
	return out
}

func referencedNames(e ast.Expr) []string {
	var out []string
	ast.Inspect(e, func(n ast.Node) bool {
		if id, ok := n.(*ast.Ident); ok {
			out = append(out, id.Name)
		}
		return true
	})
	return out
}

// qualify renders a constant's declared type as a "pkg.Name" key, or "" when
// the constant is untyped or typed by a builtin.
func qualify(fromPkg string, e ast.Expr, imports map[string]string) string {
	if e == nil {
		return ""
	}
	refs := referenced(fromPkg, e, imports)
	if len(refs) == 0 {
		return ""
	}
	return refs[0]
}

func localOf(pkg string) string {
	for _, p := range pkgs {
		if p.dir == pkg {
			return p.local
		}
	}
	return pkg
}

func header(buf *bytes.Buffer, doc string) {
	fmt.Fprintf(buf, "// Code generated by tools/gen-seam.go; DO NOT EDIT.\n\n")
	fmt.Fprintf(buf, "%s\npackage jsast\n\n", doc)
}

// writeImports emits only the aliases that are not already the package's own
// name, so the generated files read the way a human would have written them.
func writeImports(buf *bytes.Buffer, used map[string]bool) {
	buf.WriteString("import (\n")
	for _, p := range pkgs {
		if !used[p.dir] {
			continue
		}
		path := modulePath + "/internal/" + p.dir
		if p.local == p.dir {
			fmt.Fprintf(buf, "\t%q\n", path)
		} else {
			fmt.Fprintf(buf, "\t%s %q\n", p.local, path)
		}
	}
	buf.WriteString(")\n\n")
}

func renderTypes(keep map[string]bool) []byte {
	buf := &bytes.Buffer{}
	header(buf, `// Aliases, not definitions: *jsast.EDot IS *js_ast.EDot, so a type switch a
// consumer writes against these names matches nodes the parser produced.`)

	used := map[string]bool{}
	for key := range keep {
		used[strings.SplitN(key, ".", 2)[0]] = true
	}
	writeImports(buf, used)

	for _, p := range pkgs {
		var names []string
		for key := range keep {
			if pkg, name, _ := strings.Cut(key, "."); pkg == p.dir {
				names = append(names, name)
			}
		}
		if len(names) == 0 {
			continue
		}
		sort.Strings(names)
		fmt.Fprintf(buf, "// internal/%s\ntype (\n", p.dir)
		for _, n := range names {
			fmt.Fprintf(buf, "\t%s = %s.%s\n", n, p.local, n)
		}
		buf.WriteString(")\n\n")
	}
	return buf.Bytes()
}

func renderConsts(keep map[string]bool, consts []constDecl) []byte {
	buf := &bytes.Buffer{}
	header(buf, `// Re-declared rather than aliased: a type alias carries no values, and a
// consumer compares against these directly (BinOpAdd, ImportStmt, ...).
//
// A constant appears here exactly when its type appears in export.go, so the
// values of every enum a consumer can hold are nameable.`)

	byType := map[string][]constDecl{}
	for _, c := range consts {
		if c.typ == "" || !keep[c.typ] {
			continue
		}
		byType[c.typ] = append(byType[c.typ], c)
	}
	if len(byType) == 0 {
		return buf.Bytes()
	}

	used := map[string]bool{}
	for typ := range byType {
		for _, c := range byType[typ] {
			used[c.pkg] = true
		}
	}
	writeImports(buf, used)

	var typeNames []string
	for typ := range byType {
		typeNames = append(typeNames, typ)
	}
	sort.Strings(typeNames)

	for _, typ := range typeNames {
		group := byType[typ]
		// Declaration order, not alphabetical: these are iota runs, and
		// LLowest..LCall in source order is the operator precedence ladder.
		sort.Slice(group, func(i, j int) bool { return group[i].order < group[j].order })
		_, short, _ := strings.Cut(typ, ".")
		fmt.Fprintf(buf, "// %s\nconst (\n", short)
		for _, c := range group {
			fmt.Fprintf(buf, "\t%s = %s.%s\n", c.name, localOf(c.pkg), c.name)
		}
		buf.WriteString(")\n\n")
	}
	return buf.Bytes()
}

func writeFile(path string, src []byte) {
	out, err := format.Source(src)
	if err != nil {
		fmt.Fprintf(os.Stderr, "gen-seam: %s: %v\n%s\n", path, err, src)
		os.Exit(1)
	}
	check(os.WriteFile(path, out, 0o644))
	fmt.Fprintf(os.Stderr, "gen-seam: wrote %s\n", filepath.Base(path))
}

func check(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-seam:", err)
		os.Exit(1)
	}
}
