package jsast

import "testing"

// The seam is generated, so these tests pin the two properties the generator
// promises rather than any particular name it happened to emit.

// TestEnumValuesAreNameable pins the const rule: if a type is aliased into this
// package, the values it can hold are declared here too. Every line below is a
// compile-time check -- an aliased enum whose constants stopped being generated
// would fail to build, which is the failure mode the hand-written list did not
// have.
//
// ImportPhase is the case that motivated the rule: EImportCall.Phase is a
// public field on a node consumers reach from Stmts, and before the seam was
// generated none of its three values had a name here at all. ImportKind and
// SymbolKind are not reachable from Parse's result today -- File hands out
// import paths as strings and symbol names through NameOf -- but they are
// pinned too, because an exported type whose values cannot be named is a trap
// for whoever widens File next.
func TestEnumValuesAreNameable(t *testing.T) {
	var (
		_ LocalKind    = LocalConst
		_ OpCode       = BinOpAdd
		_ OpCode       = UnOpTypeof
		_ PropertyKind = PropertyGetter
		_ ScopeKind    = ScopeFunctionBody
		_ AssignTarget = AssignTargetReplace
		_ L            = LLowest

		_ ImportKind  = ImportStmt
		_ ImportKind  = ImportDynamic
		_ ImportPhase = SourcePhase
		_ ImportPhase = DeferPhase
		_ SymbolKind  = SymbolUnbound
		_ SymbolKind  = SymbolImport
	)
}

// TestConstantsMatchParsedNodes is the same guarantee checked against real
// nodes: a consumer comparing a parsed node's enum field to a constant from
// this package must get the answer the parser meant.
func TestConstantsMatchParsedNodes(t *testing.T) {
	f, errs := Parse(`const a = 1 + 2; let b = typeof a; var o = { get x() { return 1 } }`, Options{})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}

	local := f.Stmts[0].Data.(*SLocal)
	if local.Kind != LocalConst {
		t.Errorf("stmt 0 kind = %v, want LocalConst", local.Kind)
	}
	if bin, ok := local.Decls[0].ValueOrNil.Data.(*EBinary); !ok {
		t.Errorf("init = %T, want *EBinary", local.Decls[0].ValueOrNil.Data)
	} else if bin.Op != BinOpAdd {
		t.Errorf("binary op = %v, want BinOpAdd", bin.Op)
	}

	if kind := f.Stmts[1].Data.(*SLocal).Kind; kind != LocalLet {
		t.Errorf("stmt 1 kind = %v, want LocalLet", kind)
	}
	if un, ok := f.Stmts[1].Data.(*SLocal).Decls[0].ValueOrNil.Data.(*EUnary); !ok {
		t.Errorf("init = %T, want *EUnary", f.Stmts[1].Data.(*SLocal).Decls[0].ValueOrNil.Data)
	} else if un.Op != UnOpTypeof {
		t.Errorf("unary op = %v, want UnOpTypeof", un.Op)
	}

	obj := f.Stmts[2].Data.(*SLocal).Decls[0].ValueOrNil.Data.(*EObject)
	if obj.Properties[0].Kind != PropertyGetter {
		t.Errorf("property kind = %v, want PropertyGetter", obj.Properties[0].Kind)
	}
}

// TestImportPhaseIsComparable pins the specific gap the generated seam closed.
// A dynamic import with a non-literal specifier is an EImportCall, whose Phase
// field a consumer reaches straight off Stmts; naming any of its values was
// impossible while constants.go covered js_ast alone.
func TestImportPhaseIsComparable(t *testing.T) {
	f, errs := Parse(`import(spec)`, Options{})
	if len(errs) > 0 {
		t.Fatalf("Parse: %v", errs)
	}
	call, ok := f.Stmts[0].Data.(*SExpr).Value.Data.(*EImportCall)
	if !ok {
		t.Fatalf("expr = %T, want *EImportCall", f.Stmts[0].Data.(*SExpr).Value.Data)
	}
	if call.Phase != EvaluationPhase {
		t.Errorf("Phase = %v, want EvaluationPhase", call.Phase)
	}
}

// TestSeamCoversNodeKinds pins the type rule at its most load-bearing point:
// every statement and expression kind the parser can produce has a name here,
// so a consumer's type switch can be exhaustive. Written as assignments through
// the node interfaces, which only compile if each alias names the right type.
func TestSeamCoversNodeKinds(t *testing.T) {
	var (
		_ S = &SBlock{}
		_ S = &SExpr{}
		_ S = &SImport{}
		_ S = &SExportDefault{}
		_ S = &SFunction{}
		_ S = &SClass{}
		_ S = &STypeScript{}

		_ E = &EIdentifier{}
		_ E = &EDot{}
		_ E = &ECall{}
		_ E = &EImportString{}
		_ E = &ETemplate{}
		_ E = &EJSXElement{}

		_ B = &BIdentifier{}
		_ B = &BObject{}
	)
}
