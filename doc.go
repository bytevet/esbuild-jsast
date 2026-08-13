// Package jsast re-exports esbuild's internal JavaScript parser and AST.
//
// esbuild keeps its parser under internal/, so it is unreachable from any other
// module. This package is the one seam that exposes it: type ALIASES, so
// *jsast.EDot IS *js_ast.EDot and a consumer's type switches work unchanged.
//
// export.go and constants.go are generated from the vendored tree -- see
// tools/gen-seam.go for the rule that decides what appears in them. The
// hand-written surface -- Options, File, Parse -- is deliberately narrow; see
// parse.go for why.
package jsast

//go:generate go run tools/gen-seam.go
