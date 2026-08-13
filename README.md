# esbuild-jsast

esbuild's JavaScript/TypeScript parser and AST, exposed as an importable Go module.

[esbuild](https://github.com/evanw/esbuild) keeps its parser under `internal/`, which Go's
internal-package rule makes unreachable from any other module, and exposing an AST is an
[explicit non-goal](https://esbuild.github.io/faq/) of the project. This module is a vendored
subset — the 14-package dependency closure of `internal/js_parser`, nothing else — plus one
seam that re-exports it.

```go
f, errs := jsast.Parse(src, jsast.Options{TS: true})
for _, s := range f.Stmts {
    if e, ok := s.Data.(*jsast.SExpr); ok {
        // node Loc values are byte offsets into src as written
    }
}
```

The re-exports are type **aliases**, so `*jsast.EDot` *is* `*js_ast.EDot` and type switches
written against this package work on nodes the parser produced.

## Why Options is two booleans

`js_parser` is a *transforming* parser. `MinifySyntax` inlines constants and deletes dead
branches; `MinifyIdentifiers` renames symbols so a node's original source name is gone;
any `UnsupportedJSFeatures` bit turns parsing into a lowering pass; `ModeBundle` rewrites
`require`/`import` into linker shape. A caller that wants the source as written must not be
able to ask for those, so `Options` does not offer them.

## Regenerating against a newer esbuild

`export.go` and `constants.go` are generated from the vendored tree; `parse.go` is written by
hand. To move to a new upstream version, re-run the vendoring against that tag and rebuild both
generated files, then `go build ./... && go test ./...`.

## License

esbuild is MIT, © 2020 Evan Wallace — see `LICENSE.md`, which applies to everything under
`internal/`. The `jsast` package itself is offered under the same terms.
