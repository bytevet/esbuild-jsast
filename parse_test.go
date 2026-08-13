package jsast

import "testing"

func TestParseDialects(t *testing.T) {
	cases := []struct {
		name string
		src  string
		opts Options
	}{
		{"plain", `app.get("/x", function (req, res) { res.send(req.query.q) })`, Options{}},
		{"esm", "import cp from \"child_process\";\ncp.exec(x)", Options{}},
		{"ts", "const s: string = req.query.q;\nres.send(s)", Options{TS: true}},
		{"tsx", "const e = <div a={b} />;", Options{TS: true, JSX: true}},
		{"jsx", "const e = <div dangerouslySetInnerHTML={{__html: h}} />;", Options{JSX: true}},
		{"decorators", "class A { @log m() {} }", Options{TS: true}},
		{"for_await", "async function f(){ for await (const c of s) { g(c) } }", Options{}},
		{"top_level_await", `export const c = await Promise.resolve(1);`, Options{}},
		{"using", "function f(){ using r = open(); }", Options{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			f, errs := Parse(c.src, c.opts)
			if len(errs) > 0 {
				t.Fatalf("Parse: %v", errs)
			}
			if len(f.Stmts) == 0 {
				t.Fatal("no statements")
			}
		})
	}
}

// TestParseRejectsBroken pins that queued syntax errors are reported even
// though js_parser.Parse returns ok=true for them.
func TestParseRejectsBroken(t *testing.T) {
	if _, errs := Parse("function f( { 1 = ;", Options{}); len(errs) == 0 {
		t.Fatal("expected errors for malformed source")
	}
}

// TestLocIsOriginalByteOffset pins the property the whole migration rests on:
// a node's Loc is a byte offset into the source AS WRITTEN, including any
// TypeScript annotations the parser strips from the tree.
func TestLocIsOriginalByteOffset(t *testing.T) {
	src := "const name: string = req.query.q;"
	f, errs := Parse(src, Options{TS: true})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}
	local, ok := f.Stmts[0].Data.(*SLocal)
	if !ok {
		t.Fatalf("stmt 0 = %T, want *SLocal", f.Stmts[0].Data)
	}
	dot, ok := local.Decls[0].ValueOrNil.Data.(*EDot)
	if !ok {
		t.Fatalf("init = %T, want *EDot", local.Decls[0].ValueOrNil.Data)
	}
	// "req" starts at byte 21 in the ORIGINAL text; a tree built from
	// type-stripped output would report 14.
	if got := int(local.Decls[0].ValueOrNil.Loc.Start); got != 21 {
		t.Errorf("EDot target Loc = %d, want 21 (offset of `req` in the source as written)", got)
	}
	if dot.Name != "q" {
		t.Errorf("dot.Name = %q, want %q", dot.Name, "q")
	}
}

// TestNameOfRecoversSourceNames pins that identifiers are recoverable as source
// text -- the property MinifyIdentifiers would destroy.
func TestNameOfRecoversSourceNames(t *testing.T) {
	f, errs := Parse(`const cp = require("child_process"); cp.exec(c)`, Options{})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}
	expr := f.Stmts[1].Data.(*SExpr)
	call := expr.Value.Data.(*ECall)
	dot := call.Target.Data.(*EDot)
	id := dot.Target.Data.(*EIdentifier)
	if got := f.NameOf(id.Ref); got != "cp" {
		t.Errorf("NameOf = %q, want %q", got, "cp")
	}
}

// TestParserDoesNotTransform pins the Options-is-two-booleans guarantee
// behaviourally: no dead-branch removal, no constant inlining.
func TestParserDoesNotTransform(t *testing.T) {
	f, errs := Parse(`if (false) { sink(t) } const N = "x"; g(N)`, Options{})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}
	if _, ok := f.Stmts[0].Data.(*SIf); !ok {
		t.Errorf("stmt 0 = %T, want *SIf (dead branch must survive)", f.Stmts[0].Data)
	}
	call := f.Stmts[2].Data.(*SExpr).Value.Data.(*ECall)
	if _, inlined := call.Args[0].Data.(*EString); inlined {
		t.Error("const was inlined into the call; MinifySyntax must stay off")
	}
}
