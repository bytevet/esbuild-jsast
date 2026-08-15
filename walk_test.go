package jsast

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// corpus is shared by the walker tests: enough syntax to reach most node kinds,
// including the shapes whose children hide behind an intermediate struct
// (Decl, Case, Catch, Property, Arg, TemplatePart, EnumValue).
//
// It reaches 60 of the 77 node kinds. The remainder are not reachable through
// this package at all, rather than merely untested: EImportIdentifier,
// ERequireString, ERequireResolveString, SLazyExport, ENameOfSymbol,
// EInlinedEnum and EAnnotation are produced by bundling and minification, which
// Options deliberately cannot switch on; EJSXElement, EJSXText, SEnum,
// SNamespace, STypeScript and SExportEquals are lowered to plain JS shapes
// while parsing; BMissing and EMissing only appear in trees Parse rejects.
var corpus = []struct {
	name string
	src  string
	opts Options
}{
	{"decls", `const a = 1, {b, c: [d]} = e; let f; var g = h ?? i`, Options{}},
	{"control", `if (a) { b() } else while (c) { d(); for (const e of g) h(e) }`, Options{}},
	{"switch_try", `switch (a) { case 1: b(); break; default: c() } try { d() } catch (e) { f(e) } finally { g() }`, Options{}},
	{"functions", `function a(b, c = 1, ...d) { return b } const e = (f) => f + 1; async function* h() { yield 1 }`, Options{}},
	{"classes", `class A extends B { static x = 1; #y; get z() { return 1 }; static { init() } m(n) { super.m(n) } }`, Options{}},
	{"exprs", "a?.b?.[c]?.(d); e = `t${f}u${g}v`; new h(i); [j, ...k]; ({l, ...m}); n ? o : p; typeof q; -r; s++", Options{}},
	{"modules", `import a, {b as c} from "d"; export {a}; export default e; export * from "f"; export {i} from "j"; import("g"); import(h)`, Options{}},
	{"ts", `enum E { A = 1, B } namespace N { export const x: number = 1 } const y = <T,>(z: T): T => z`, Options{TS: true}},
	{"jsx", `const a = <div x={y} {...z}><b>{c}</b>{"d"}</div>`, Options{JSX: true}},
	{"misc", `label: for (;;) { break label } do { a() } while (b); with (c) { d } debugger; "use strict"`, Options{}},
	{"more_stmts", `for (const a in b) { if (a) continue; else throw new Error(a) } for (var c in d) e(c); ;`, Options{}},
	{"more_exprs", `a = true; b = false; c = /re/g; d = 1n; e = void 0; f = this; g = class extends h {}; i = undefined`, Options{}},
	{"meta", `async function a() { await b; return import.meta.url } function C() { return new.target } export const d = await e`, Options{}},
}

// TestWalkerMatchesReflection is the test that makes a generated traversal
// trustworthy. A walker that silently skips a subtree looks correct on every
// input you thought to check, so checking hand-picked expectations proves
// almost nothing.
//
// Instead the generated walker is compared against an independent oracle: a
// reflection walk that descends every exported field of every value without
// knowing anything about node kinds. It is far too slow to ship, and it cannot
// be wrong about which fields exist. Any field the generator failed to emit
// shows up here as a node the oracle reached and the walker did not.
func TestWalkerMatchesReflection(t *testing.T) {
	for _, c := range corpus {
		t.Run(c.name, func(t *testing.T) {
			f, errs := Parse(c.src, c.opts)
			if len(errs) > 0 {
				t.Fatalf("Parse: %v", errs)
			}

			var got []string
			Inspect(f.Stmts, func(n Node) bool {
				got = append(got, fmt.Sprintf("%d:%T", n.Loc.Start, n.Data))
				return true
			})

			var want []string
			for i := range f.Stmts {
				reflectWalk(reflect.ValueOf(&f.Stmts[i]).Elem(), &want, map[any]bool{})
			}

			if len(got) == 0 {
				t.Fatal("walker visited nothing")
			}
			if strings.Join(got, "\n") != strings.Join(want, "\n") {
				t.Errorf("walker and reflection disagree\n%s", diff(want, got))
			}
		})
	}
}

// TestInspectSkipsChildren pins the false-means-prune contract. Without it a
// visitor searching for top-level calls would have to filter out every nested
// one by hand.
func TestInspectSkipsChildren(t *testing.T) {
	f, errs := Parse(`a(b(c(d)))`, Options{})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}

	var all, pruned int
	f.Inspect(func(Node) bool { all++; return true })
	f.Inspect(func(n Node) bool {
		pruned++
		_, isCall := n.Data.(*ECall)
		return !isCall // descend until the first call, then stop
	})

	if all <= pruned {
		t.Fatalf("pruning visited %d of %d nodes; expected fewer", pruned, all)
	}
	// SExpr, then the outermost ECall, and nothing under it.
	if pruned != 2 {
		t.Errorf("pruned walk visited %d nodes, want 2 (SExpr + outermost ECall)", pruned)
	}
}

// TestInspectVisitsParentBeforeChild pins pre-order, which a visitor tracking
// enclosing context depends on.
func TestInspectVisitsParentBeforeChild(t *testing.T) {
	f, errs := Parse(`a(b)`, Options{})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}
	var order []string
	f.Inspect(func(n Node) bool {
		order = append(order, fmt.Sprintf("%T", n.Data))
		return true
	})
	want := []string{"*js_ast.SExpr", "*js_ast.ECall", "*js_ast.EIdentifier", "*js_ast.EIdentifier"}
	if strings.Join(order, ",") != strings.Join(want, ",") {
		t.Errorf("order = %v, want %v", order, want)
	}
}

var (
	stmtType    = reflect.TypeOf(Stmt{})
	exprType    = reflect.TypeOf(Expr{})
	bindingType = reflect.TypeOf(Binding{})
)

// reflectWalk is the oracle: it descends everything reachable through exported
// fields and records each node wrapper it meets, in the same pre-order the
// generated walker uses.
func reflectWalk(v reflect.Value, out *[]string, seen map[any]bool) {
	switch v.Kind() {
	case reflect.Struct:
		switch v.Type() {
		case stmtType, exprType, bindingType:
			data := v.FieldByName("Data")
			if data.IsNil() {
				return
			}
			*out = append(*out, fmt.Sprintf("%d:%T", v.FieldByName("Loc").FieldByName("Start").Int(), data.Interface()))
			reflectWalk(data, out, seen)
			return
		}
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).IsExported() {
				reflectWalk(v.Field(i), out, seen)
			}
		}
	case reflect.Slice, reflect.Array:
		for i := 0; i < v.Len(); i++ {
			reflectWalk(v.Index(i), out, seen)
		}
	case reflect.Pointer:
		if v.IsNil() {
			return
		}
		// Scope carries a Parent pointer, so the object graph is not a tree
		// even though the syntax tree is.
		if key := v.Interface(); seen[key] {
			return
		} else {
			seen[key] = true
		}
		reflectWalk(v.Elem(), out, seen)
	case reflect.Interface:
		if !v.IsNil() {
			reflectWalk(v.Elem(), out, seen)
		}
	}
}

func diff(want, got []string) string {
	var b strings.Builder
	for i := 0; i < max(len(want), len(got)); i++ {
		w, g := "<missing>", "<missing>"
		if i < len(want) {
			w = want[i]
		}
		if i < len(got) {
			g = got[i]
		}
		mark := "  "
		if w != g {
			mark = "->"
		}
		fmt.Fprintf(&b, "%s %-44s reflection=%s\n", mark, "walker="+g, w)
		if mark == "->" {
			break
		}
	}
	return b.String()
}
