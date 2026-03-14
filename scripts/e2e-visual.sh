#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
tapes_dir="$repo_root/e2e/tests"

if [ "$#" -ne 0 ]; then
  printf 'usage: %s\n' "$(basename "$0")" >&2
  exit 1
fi

if ! command -v vhs >/dev/null 2>&1; then
  printf 'missing dependency: vhs\n' >&2
  exit 1
fi

set -- "$tapes_dir"/*.tape
if [ "$1" = "$tapes_dir/*.tape" ]; then
  printf 'no tapes found in %s\n' "$tapes_dir" >&2
  exit 1
fi

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/verti-e2e-visual-XXXXXX")
trap 'rm -rf "$tmpdir"' EXIT INT TERM

home_dir="$tmpdir/home"
repo_dir="$tmpdir/test-repo"
bin_path="$tmpdir/verti"

mkdir -p "$home_dir" "$repo_root/e2e/visual"

(
  cd "$repo_root"
  go build -o "$bin_path" ./cmd/verti
)

HOME="$home_dir" \
GIT_EDITOR=true \
TEST_REPO_DIR="$repo_dir" \
VERTI_BIN="$bin_path" \
  "$repo_root/scripts/test-repo.sh"

for tape_path in "$@"; do
  scenario_name=$(basename "$tape_path" .tape)
  gif_path="$repo_root/e2e/visual/$scenario_name.gif"
  visual_tape="$tmpdir/$scenario_name.visual.tape"

  awk -v gif_path="e2e/visual/$scenario_name.gif" '
    /^Output / { next }
    /^Set TypingSpeed / { print "Set TypingSpeed 80ms"; next }
    /^Sleep 50ms$/ { print "Sleep 400ms"; next }
    /^Sleep 100ms$/ { print "Sleep 900ms"; next }
    { print }
    END { print ""; print "Output " gif_path }
  ' "$tape_path" > "$visual_tape"

  (
    cd "$repo_root"
    HOME="$home_dir" \
    GIT_EDITOR=true \
    PATH="$(dirname "$bin_path"):$PATH" \
    E2E_TEST_REPO="$repo_dir" \
      vhs "$visual_tape"
  )

  printf 'wrote %s\n' "$gif_path"
done
