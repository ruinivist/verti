#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
name="${NAME:-}"
force="${FORCE:-0}"

usage() {
  printf 'usage: make e2e-record NAME=<scenario> [FORCE=1]\n' >&2
}

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

if [ -z "$name" ]; then
  usage
  exit 1
fi

case "$name" in
  *[!A-Za-z0-9_-]*)
    fail "invalid NAME: use only letters, numbers, underscores, or dashes"
    ;;
esac

for cmd in bash go script sed tail mktemp cp od tr truncate; do
  if ! command -v "$cmd" >/dev/null 2>&1; then
    fail "missing dependency: $cmd"
  fi
done

keys_path="$repo_root/e2e/tests/$name.keys"
golden_path="$repo_root/e2e/tests/$name.golden.out"

if [ "$force" != "1" ]; then
  for path in "$keys_path" "$golden_path"; do
    if [ -e "$path" ]; then
      fail "refusing to overwrite $path (rerun with FORCE=1 to replace it)"
    fi
  done
fi

tmpdir=$(mktemp -d "${TMPDIR:-/tmp}/verti-e2e-record-XXXXXX")
keep_tmp=0

cleanup() {
  if [ "$keep_tmp" -eq 0 ]; then
    rm -rf "$tmpdir"
  else
    printf 'kept temp dir: %s\n' "$tmpdir" >&2
  fi
}

trap cleanup EXIT INT TERM

home_dir="$tmpdir/home"
repo_dir="$tmpdir/test-repo"
replay_home_dir="$tmpdir/replay-home"
replay_repo_dir="$tmpdir/replay-repo"
bin_path="$tmpdir/verti"
raw_in="$tmpdir/$name.raw.in"
raw_out="$tmpdir/$name.raw.out"
clean_in="$tmpdir/$name.keys"
replay_raw_out="$tmpdir/$name.replay.raw.out"
clean_out="$tmpdir/$name.golden.out"

mkdir -p "$home_dir" "$repo_root/e2e/tests"

printf '== build verti ==\n' >&2
(
  cd "$repo_root"
  go build -o "$bin_path" ./cmd/verti
)

printf '== setup test repo ==\n' >&2
HOME="$home_dir" \
TEST_REPO_DIR="$repo_dir" \
VERTI_BIN="$bin_path" \
  "$repo_root/scripts/test-repo.sh" >&2

printf '== record %s ==\n' "$name" >&2
printf 'Recording into a shell rooted at %s\n' "$repo_dir" >&2
printf 'Run the scenario, then press Ctrl+D to finish recording.\n' >&2

HOME="$home_dir" \
HISTFILE=/dev/null \
PATH="$tmpdir:$PATH" \
TERM=xterm-256color \
E2E_TEST_REPO="$repo_dir" \
  script -q -e -E auto -I "$raw_in" -O "$raw_out" -c "$repo_root/scripts/e2e-shell.sh"

printf 'Save recorded tape for %s? [y/N] ' "$name" >&2
IFS= read -r save_reply || save_reply=""
case "$save_reply" in
  y|Y|yes|YES|Yes)
    ;;
  *)
    printf 'discarded recording for %s\n' "$name" >&2
    exit 0
    ;;
esac

tail -n +2 "$raw_in" | sed '$d' > "$clean_in"

if [ ! -s "$clean_in" ]; then
  keep_tmp=1
  fail "recording did not capture any keystrokes"
fi

last_byte=$(tail -c 1 "$clean_in" | od -An -tx1 | tr -d ' \n')
if [ "$last_byte" = "0a" ]; then
  truncate -s -1 "$clean_in"
fi

printf '== replay %s ==\n' "$name" >&2
mkdir -p "$replay_home_dir"
HOME="$replay_home_dir" \
TEST_REPO_DIR="$replay_repo_dir" \
VERTI_BIN="$bin_path" \
  "$repo_root/scripts/test-repo.sh" >&2

HOME="$replay_home_dir" \
HISTFILE=/dev/null \
PATH="$tmpdir:$PATH" \
TERM=xterm-256color \
E2E_TEST_REPO="$replay_repo_dir" \
  script -q -e -E never -O "$replay_raw_out" -c "$repo_root/scripts/e2e-shell.sh" < "$clean_in"

tail -n +2 "$replay_raw_out" | sed '$d' > "$clean_out"

cp "$clean_in" "$keys_path"
cp "$clean_out" "$golden_path"

printf 'wrote %s\n' "$keys_path" >&2
printf 'wrote %s\n' "$golden_path" >&2
