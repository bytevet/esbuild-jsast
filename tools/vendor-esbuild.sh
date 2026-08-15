#!/bin/sh
#
# Regenerates internal/ from an upstream esbuild tag.
#
# internal/ is not hand-maintained: every file under it is upstream's file for
# the pinned tag, byte for byte, with one substitution -- the import path
# prefix, which Go's internal-package rule forces (a module cannot import
# github.com/evanw/esbuild/internal/..., so the tree has to be re-rooted under
# this module's path). A submodule or subtree of upstream would keep the
# original paths and therefore would not compile; see README.md.
#
# The package set is derived, not listed: it is the dependency closure of
# internal/js_parser, unioned across every GOOS this module supports, so a new
# upstream dependency is picked up instead of silently missed.
#
#   tools/vendor-esbuild.sh                  regenerate at the pinned version
#   tools/vendor-esbuild.sh --version v0.29.0  move the pin, then regenerate
#   tools/vendor-esbuild.sh --check           verify internal/ matches the pin
#
# --check writes nothing and exits non-zero on any drift; it is the CI gate
# that keeps "vendored copy" from turning into "fork nobody can audit".

set -eu

repo=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
pin="$repo/ESBUILD_VERSION"
upstream_repo=https://github.com/evanw/esbuild.git
upstream_mod=github.com/evanw/esbuild
local_mod=github.com/bytevet/esbuild-jsast
root_pkg=./internal/js_parser
platforms='linux/amd64 darwin/arm64 windows/amd64 js/wasm'

version=
check=0
while [ $# -gt 0 ]; do
	case $1 in
	--check) check=1 ;;
	--version)
		[ $# -ge 2 ] || { echo "vendor-esbuild: --version needs a tag" >&2; exit 2; }
		version=$2
		shift
		;;
	--version=*) version=${1#--version=} ;;
	*)
		echo "vendor-esbuild: unknown argument: $1" >&2
		exit 2
		;;
	esac
	shift
done

if [ -n "$version" ] && [ "$check" -eq 1 ]; then
	echo "vendor-esbuild: --check verifies the recorded pin; drop --version" >&2
	exit 2
fi
if [ -z "$version" ]; then
	version=$(sed -n 's/^version=//p' "$pin")
	[ -n "$version" ] || { echo "vendor-esbuild: no version= line in $pin" >&2; exit 1; }
fi

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT INT TERM

echo "vendor-esbuild: fetching esbuild $version" >&2
git -c advice.detachedHead=false clone --quiet --depth 1 --branch "$version" \
	"$upstream_repo" "$work/upstream"
commit=$(git -C "$work/upstream" rev-parse HEAD)

# When the pin is being verified rather than moved, a tag that has been
# repointed upstream is drift too -- report it instead of vendoring over it.
if [ "$check" -eq 1 ]; then
	pinned_commit=$(sed -n 's/^commit=//p' "$pin")
	if [ -n "$pinned_commit" ] && [ "$pinned_commit" != "$commit" ]; then
		echo "vendor-esbuild: $version now resolves to $commit," >&2
		echo "  but $pin records $pinned_commit" >&2
		exit 1
	fi
fi

# Dependency closure of the parser, per platform, because build tags decide
# which files -- and so which imports -- a package contributes.
pkgs=
for platform in $platforms; do
	found=$(cd "$work/upstream" && GOWORK=off GOFLAGS=-mod=mod \
		GOOS=${platform%/*} GOARCH=${platform#*/} go list -deps "$root_pkg" |
		sed -n "s|^$upstream_mod/internal/||p")
	pkgs="$pkgs $found"
done
pkgs=$(printf '%s\n' $pkgs | sort -u)
[ -n "$pkgs" ] || { echo "vendor-esbuild: empty package closure" >&2; exit 1; }

# Build the tree beside the real one, so a failure partway through cannot
# leave internal/ half-updated.
staged="$work/internal"
mkdir -p "$staged"
for pkg in $pkgs; do
	src="$work/upstream/internal/$pkg"
	[ -d "$src" ] || { echo "vendor-esbuild: $pkg missing from $version" >&2; exit 1; }
	mkdir -p "$staged/$pkg"
	for file in "$src"/*.go; do
		case $file in
		*_test.go) continue ;;
		esac
		sed "s|$upstream_mod/internal/|$local_mod/internal/|g" \
			"$file" >"$staged/$pkg/$(basename "$file")"
	done
	# Anything other than .go here (testdata, embedded assets) would be dropped
	# silently, so refuse rather than vendor a package that is missing pieces.
	extra=$(find "$src" -mindepth 1 ! -name '*.go' | head -n 1)
	[ -z "$extra" ] || { echo "vendor-esbuild: $pkg has non-Go files ($extra)" >&2; exit 1; }
done
cp "$work/upstream/LICENSE.md" "$staged.LICENSE.md"

if [ "$check" -eq 1 ]; then
	status=0
	diff -ru "$repo/internal" "$staged" || status=1
	diff -u "$repo/LICENSE.md" "$staged.LICENSE.md" || status=1
	if [ "$status" -ne 0 ]; then
		echo "vendor-esbuild: internal/ does not match esbuild $version" >&2
		echo "  run tools/vendor-esbuild.sh to regenerate" >&2
		exit 1
	fi
	echo "vendor-esbuild: internal/ matches esbuild $version ($commit)" >&2
	exit 0
fi

rm -rf "$repo/internal"
cp -R "$staged" "$repo/internal"
cp "$staged.LICENSE.md" "$repo/LICENSE.md"
printf 'version=%s\ncommit=%s\n' "$version" "$commit" >"$pin"

printf 'vendor-esbuild: vendored %s (%s):\n' "$version" "$commit" >&2
printf '  %s\n' $pkgs >&2

# export.go, constants.go and walk.go are functions of the tree that was just
# replaced, so they are stale by definition until regenerated. Doing it here
# rather than leaving it to the caller is what makes a bump one command; an
# upstream release that adds a node kind or a constant is picked up instead of
# silently missed, because a stale generated file still compiles.
if ! (cd "$repo" && go generate ./...); then
	echo "vendor-esbuild: regeneration failed against $version" >&2
	exit 1
fi

# Build, vet and test after regenerating, not before: the seam may legitimately
# have changed shape, and what matters is whether the result is sound.
if ! (cd "$repo" && go build ./... && go vet ./... && go test ./...); then
	echo >&2
	echo "vendor-esbuild: the module does not build or pass tests against $version." >&2
	echo "  If the failure is in parse.go or a test, upstream changed behaviour this" >&2
	echo "  package depends on -- that is the signal to read, not to work around." >&2
	exit 1
fi

if ! (cd "$repo" && git diff --quiet -- export.go constants.go walk.go); then
	echo >&2
	echo "vendor-esbuild: the generated seam/walker CHANGED with this bump." >&2
	echo "  Review the diff: added names are new upstream surface, removed names" >&2
	echo "  are a breaking change for consumers of this package." >&2
fi
