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

## Walking the tree

`Inspect` visits every statement, expression and binding in depth-first pre-order. Returning
`false` prunes that node's children.

```go
f.Inspect(func(n jsast.Node) bool {
    call, ok := n.Data.(*jsast.ECall)
    if !ok {
        return true
    }
    if dot, ok := call.Target.Data.(*jsast.EDot); ok && dot.Name == "exec" {
        report(n.Loc) // n.Loc is the byte offset of the call
    }
    return true
})
```

`Node` pairs a node kind with the `Loc` of the wrapper it was reached through, since `Loc` lives
on `Stmt`/`Expr`/`Binding` rather than on the kind itself. Fields following esbuild's `OrNil`
convention are skipped when empty, so `n.Data` is never nil.

## Why Options is two booleans

`js_parser` is a *transforming* parser. `MinifySyntax` inlines constants and deletes dead
branches; `MinifyIdentifiers` renames symbols so a node's original source name is gone;
any `UnsupportedJSFeatures` bit turns parsing into a lowering pass; `ModeBundle` rewrites
`require`/`import` into linker shape. A caller that wants the source as written must not be
able to ask for those, so `Options` does not offer them.

## The one exception: ExperimentalDecorators

A decorator on a **parameter** — `list(@Req() req: Request)` — is a hard parse error unless
TypeScript's legacy decorators are enabled, and the error rejects the whole file. That makes
NestJS and Angular sources unparseable, so `Options.ExperimentalDecorators` exists to admit
them.

It is the one knob here that changes what the tree says, so it is off by default. `js_parser`
has no parse-and-discard path — accepting the syntax and lowering it are gated on the same
flag — so switching it on lowers *every* experimental decorator in the file, not just the
parameter ones:

| `@Controller('x') class C { @Get() list() {}; @Inject() svc }` | class decorators | property decorators | `__decorate*` calls |
| --- | --- | --- | --- |
| `ExperimentalDecorators: false` | 1 | 2 | 0 |
| `ExperimentalDecorators: true`  | 0 | 0 | 3 |

Decorators stop appearing as `Class.Decorators` / `Property.Decorators` and come back as
`__decorateClass` / `__decorateParam` calls after the class.

Worse, the flag is not a superset: `(@dec class {})` and `class C { @dec #x = 1 }` **parse
with it off and fail with it on**. Switching it on globally trades one set of parse failures
for another, on top of losing decorator nodes everywhere.

So parse with it off and retry only when it would help. `IsExperimentalDecoratorError` is the
predicate for that — the diagnostic carries no message ID upstream, so it has to be matched by
text, and keeping that match here behind a test means an upstream reword breaks a test at
re-vendoring time rather than silently disabling the retry in your code:

```go
f, errs := jsast.Parse(src, jsast.Options{TS: true})
if jsast.IsExperimentalDecoratorError(errs) {
    f, errs = jsast.Parse(src, jsast.Options{TS: true, ExperimentalDecorators: true})
}
```

## Syncing a newer esbuild

One command. It clones the tag, re-derives the package closure, rewrites the import prefix,
regenerates the seam and walker, then builds, vets and tests:

```sh
tools/vendor-esbuild.sh --version v0.28.2   # move the pin and sync
tools/vendor-esbuild.sh                     # re-sync at the pinned version
tools/vendor-esbuild.sh --check             # verify internal/ matches the pin, change nothing
```

`ESBUILD_VERSION` records the tag and its resolved commit. `--check` fails on any drift,
including a tag repointed upstream, which makes it the gate to run in CI.

Three things to read after a sync, in order of how quietly they fail:

1. **The build or tests failed.** Upstream changed behaviour this package depends on. That is
   the signal to read, not to work around — `parse.go` is the only hand-written coupling.
2. **`export.go` / `constants.go` / `walk.go` changed.** The script says so explicitly. Added
   names are new upstream surface; *removed* names are a breaking change for your consumers.
3. **Nothing changed and everything passed.** Still worth a glance at the vendored diff — it is
   upstream's diff, since every file is upstream's byte for byte apart from the import prefix.

`doc.go` and `parse.go` are hand-written and never regenerated, so anything you have added
there — `Options`, `Parse`, `IsExperimentalDecoratorError` — survives a sync. It is the tests
that tell you whether it still *works*, which is why the script runs them.

CI (`.github/workflows/ci.yml`) enforces all of this on every push and pull request:

| job | gate |
| --- | --- |
| `test` | gofmt, build, vet, `go test -race` — on Go 1.25.x (the floor in `go.mod`) and stable |
| `generated` | `go generate` must produce no diff |
| `vendored` | `tools/vendor-esbuild.sh --check` |

`generated` is the one that earns its keep: a stale `export.go` still *compiles*, so an upstream
release that adds a node kind or a constant would otherwise leave CI green while the new surface
silently went missing. `vendored` is a separate job because it clones esbuild over the network —
when it fails you want to see at a glance whether the tree drifted or the network did.

The seam is *derived*, not listed, because a list is the wrong shape for a file whose contents
are a function of `internal/`: an upstream release that **adds** a type or constant does not
break the build, so a stale list stays green while quietly withholding the new surface. The rule
is two lines:

- a type is aliased if `js_ast` exports it, or if it is reachable from one of those through an
  exported field, transitively
- a constant is re-declared if its type is aliased — a value is nameable exactly when the thing
  that holds it is

That second rule is what makes the seam self-consistent. It also settles the edge cases without
an exclusion list: `js_ast.NSExportPartIndex` drops out on its own (a bundler part index typed
`uint32`, not an AST enum), while `ast`'s enum values come in, since `EImportCall.Phase` and
friends are fields a consumer reaches straight off `Stmts`.

Two small lists in the generator carry the judgment calls that reachability can't make —
`neverExport` for renamer state that sits beside `Symbol` without being part of it, and
`alwaysExport` for names published before the seam was derived. Both are commented with why.

`walk.go` is generated the same way, by `tools/gen-walk.go`: a field is traversed when its type
can carry a `Stmt`, `Expr` or `Binding` — directly, through a slice or pointer, or through an
intermediate struct like `Decl` or `Property`. Hand-writing that is a bad bet, because a missed
field makes the walker skip a subtree silently, and upstream adds fields monthly.

The walker is checked against an independent oracle rather than against expectations. A
reflection walk in `walk_test.go` descends every exported field of every value, knowing nothing
about node kinds; the generated walker must agree with it node for node across the test corpus.
Deleting one traversal line from `walk.go` makes that test fail and names the node it stopped
at. The oracle is ~170x slower, which is the reason it stays in the test and the generated code
ships.

## License

esbuild is MIT, © 2020 Evan Wallace — see `LICENSE.md`, which applies to everything under
`internal/`. The `jsast` package itself is offered under the same terms.
