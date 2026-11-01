#!/usr/bin/env bash
# Renders the Go snippets of the landing page (site/snippets/*/main.go) to the
# PNGs in site/src/assets/landing/, light and dark, with `tela snapshot` at the
# size of each App.
#
#   scripts/render-snippets.sh          render and overwrite the committed PNGs
#   scripts/render-snippets.sh --check  render into a temp dir and compare with
#                                       the committed PNGs; fail on any difference,
#                                       a stale extra PNG included
#
# Tela renders deterministically, so the PNGs must be byte-identical.
# No window is ever opened: snapshot draws on the headless backend.
set -euo pipefail
export CGO_ENABLED=0

check=0
case "${1:-}" in
  "") ;;
  --check) check=1 ;;
  *) echo "usage: $0 [--check]" >&2; exit 2 ;;
esac

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
site="$(dirname "$here")"
root="$(dirname "$site")"
assets="$site/src/assets/landing"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

# Build tela once; the snippets module resolves the framework from this checkout.
(cd "$root" && go build -o "$tmp/tela" ./cmd/tela)

out="$assets"
if [ "$check" = 1 ]; then out="$tmp/out"; fi
mkdir -p "$out"

# Render every PNG in parallel, at most one job per CPU. Each job logs to its own
# file, so the output stays readable; the logs are printed in order afterwards.
jobs_max="$( (nproc || sysctl -n hw.ncpu || echo 4) 2>/dev/null | head -n 1)"
mkdir -p "$tmp/log"
cd "$site/snippets"

declare -a names=() pids=()
render() { # render <name> <tela snapshot flags...> <package>
  local name="$1"; shift
  "$tmp/tela" snapshot "$@" >"$tmp/log/$name.log" 2>&1 &
  names+=("$name")
  pids+=($!)
  # Stay within the CPU count: wait for any running job before starting another.
  while [ "$(jobs -rp | wc -l)" -ge "$jobs_max" ]; do wait -n || true; done
}

for dir in */main.go; do
  p="${dir%/main.go}"
  render "$p" --scale 2 -o "$out/$p.png" "./$p"
  # theming sets Scheme on both of its panels: its dark render is identical.
  if [ "$p" != theming ]; then
    render "$p-dark" --dark --scale 2 -o "$out/$p-dark.png" "./$p"
  fi
done

failed=0
for i in "${!pids[@]}"; do
  if ! wait "${pids[$i]}"; then
    echo "render failed: ${names[$i]}" >&2
    cat "$tmp/log/${names[$i]}.log" >&2
    failed=1
  else
    cat "$tmp/log/${names[$i]}.log"
  fi
done
if [ "$failed" = 1 ]; then
  echo "Some snippet renders failed (see above)." >&2
  exit 1
fi

if [ "$check" = 1 ]; then
  # diff -rq also fails on a stale PNG that no snippet renders any more.
  if ! diff -rq "$out" "$assets" >&2; then
    echo "The snippet PNGs are stale: run site/scripts/render-snippets.sh and commit them." >&2
    exit 1
  fi
  echo "snippet PNGs are up to date"
fi
