# Split the vendored tree into a nested module

Target: `github.com/bytevet/esbuild-jsast/internal/mirror`, versioned independently,
required by the parent module. Public API of `jsast` must not change.

## Phase 0 — de-risk (blocking)

A module path containing `/internal/` resolves fine locally (verified against a
file-based proxy), but the tag→version mapping on a real host is unproven. If
this phase fails, the design is dead and nothing has been touched yet.

- [ ] Create scratch branch with a minimal `internal/mirror/go.mod` (module path + `go` directive only)
- [ ] Push tag `internal/mirror/v0.0.1-probe`
- [ ] From a clean module cache, resolve it **direct, bypassing the public proxy and sumdb**:
      `GOPRIVATE='github.com/bytevet/*' GOFLAGS=-mod=mod go mod download github.com/bytevet/esbuild-jsast/internal/mirror@v0.0.1-probe`
- [ ] Confirm the tag prefix maps to the subdirectory module (not the repo root)
- [ ] Delete probe tag and scratch branch
- [ ] **Decision gate:** if the path is rejected, stop and report — fall back to a public
      `/mirror` path (loses the internal-only guarantee) or abandon the split

## Phase 1 — restructure

- [ ] `git mv internal/<pkg> internal/mirror/<pkg>` for all 14 packages
- [ ] Add `internal/mirror/go.mod` (module path, `go 1.25.0`, `require golang.org/x/sys v0.47.0`)
- [ ] Move `go.sum` entries for x/sys into the mirror module; drop from parent
- [ ] Rewrite import prefix `…/internal/` → `…/internal/mirror/` inside the moved tree only
- [ ] Add `internal/mirror/ESBUILD_VERSION` recording `v0.28.1` + commit `bb9db84…`
- [ ] Update `modulePath` / package-dir constants in `tools/gen-seam.go` and `tools/gen-walk.go`
- [ ] Regenerate: `go generate ./...`
- [ ] Update parent `go.mod`: drop `golang.org/x/sys`, add the mirror requirement
- [ ] Add `go.work` (`use .`, `use ./internal/mirror`) so the tree builds before any tag exists

## Phase 2 — verify

- [ ] `go generate ./...` is idempotent (no diff on second run)
- [ ] Public surface unchanged: type/const name sets identical to pre-split, only
      import paths in the generated files differ
- [ ] Walker oracle test still passes (`TestWalkerMatchesReflection`)
- [ ] **Completion gate:** `gofmt -l .` empty, `go vet ./...` clean, `go test ./...` green —
      run in both the parent and the mirror module, zero errors

## Phase 3 — publish (two-step, order matters)

- [ ] Commit and push the restructure
- [ ] Tag `internal/mirror/v0.1.0`, push the tag
- [ ] Pin parent's requirement to `v0.1.0`; `GOWORK=off go mod tidy`
- [ ] Verify `GOWORK=off go build ./...` resolves the mirror from the network
- [ ] Commit the pinned `go.mod` / `go.sum`

## Phase 4 — guardrails

- [ ] Add `.github/workflows/ci.yml`: build + vet + test with **`GOWORK=off`** so a
      forgotten requirement bump fails instead of passing on the workspace
- [ ] Matrix Go 1.26.x (current stable) and 1.25.x (the declared minimum)
- [ ] Document the two-step release order in README

## Out of scope

Pruning the 6 never-executed packages, and re-landing the vendor script. Both
compose with this and neither blocks it.
