package jsast

import (
	"github.com/bytevet/esbuild-jsast/internal/config"
	"github.com/bytevet/esbuild-jsast/internal/helpers"
	"github.com/bytevet/esbuild-jsast/internal/js_ast"
	"github.com/bytevet/esbuild-jsast/internal/js_parser"
	"github.com/bytevet/esbuild-jsast/internal/logger"
)

// UTF16ToString converts an EString.Value / ETemplate cooked part to a Go
// string. Required, not a convenience: esbuild stores string literals as
// []uint16 because JS strings are UTF-16.
func UTF16ToString(text []uint16) string { return helpers.UTF16ToString(text) }

// Options is the entire configuration surface this package exposes, and it is
// two booleans on purpose.
//
// js_parser is a TRANSFORMING parser, not a pure one: config.Options carries
// knobs that change what the tree SAYS, not merely how it is printed --
// MinifySyntax inlines constants and deletes dead branches, MinifyIdentifiers
// renames symbols so OriginalName stops being the source name, any
// UnsupportedJSFeatures bit turns the parse into a lowering pass, and ModeBundle
// rewrites require/import into linker shape. A consumer that wants the source
// as written must not be able to ask for any of that, so the type does not let
// it. The zero value is a plain-JavaScript parse.
type Options struct {
	TS  bool // parse TypeScript syntax
	JSX bool // parse JSX with the classic React.createElement pragma
}

// File is one parsed source file.
//
// Stmts is the AST's Parts concatenated in source order: Parts are a bundler
// concept (a unit of tree-shaking granularity), and a consumer that only reads
// the tree should never have to know they exist.
type File struct {
	Stmts   []Stmt
	Imports []string // import-record paths, indexed by SImport.ImportRecordIndex

	symbols []Symbol
}

// NameOf returns a symbol's original source name -- what an EIdentifier,
// EImportIdentifier or BIdentifier was written as. Node identifiers are Refs
// into this table, never strings, so this is the only way back to source text.
func (f *File) NameOf(r Ref) string {
	if int(r.InnerIndex) >= len(f.symbols) {
		return ""
	}
	return f.symbols[r.InnerIndex].OriginalName
}

// Inspect walks every node in the file in depth-first order. See Inspect for
// the traversal's contract.
func (f *File) Inspect(fn func(Node) bool) { Inspect(f.Stmts, fn) }

// Error is a parse diagnostic. Line is 1-based; Column is a 0-based BYTE offset
// within the line, matching esbuild's own convention.
type Error struct {
	Text   string
	Line   int
	Column int
}

// Parse parses one JavaScript/TypeScript source buffer.
//
// A nil *File is returned only alongside a non-empty error slice. Success is
// NOT js_parser.Parse's ok alone: ok goes false only on a lexer panic, while
// ordinary syntax errors are queued on the log and leave ok true -- so a parse
// with queued errors would otherwise be mistaken for a good tree.
func Parse(contents string, opts Options) (*File, []Error) {
	log := logger.NewDeferLog(logger.DeferLogNoVerboseOrDebug, nil)
	co := config.Options{}
	if opts.TS {
		co.TS = config.TSOptions{Parse: true}
	}
	if opts.JSX {
		co.JSX = config.JSXOptions{Parse: true}
	}
	src := logger.Source{Contents: contents}

	tree, ok := js_parser.Parse(log, src, js_parser.OptionsFromConfig(&co))
	msgs := log.Done()
	if errs := collectErrors(msgs); len(errs) > 0 || !ok {
		if len(errs) == 0 {
			errs = []Error{{Text: "parse failed"}}
		}
		return nil, errs
	}

	f := &File{symbols: tree.Symbols}
	for _, p := range tree.Parts {
		f.Stmts = append(f.Stmts, p.Stmts...)
	}
	for _, r := range tree.ImportRecords {
		f.Imports = append(f.Imports, r.Path.Text)
	}
	return f, nil
}

// collectErrors keeps only errors. esbuild warns ("duplicate key", "unused
// import") on files that parse perfectly well, and a warning must never cost
// the file.
func collectErrors(msgs []logger.Msg) []Error {
	var errs []Error
	for _, m := range msgs {
		if m.Kind != logger.Error {
			continue
		}
		e := Error{Text: m.Data.Text}
		if loc := m.Data.Location; loc != nil {
			e.Line, e.Column = loc.Line, loc.Column
		}
		errs = append(errs, e)
	}
	return errs
}

// compile-time proof the alias seam actually works: a type switch written
// against this package's names must accept nodes the internal parser produced.
var _ = func(s Stmt) bool { _, ok := s.Data.(*js_ast.SExpr); return ok }
