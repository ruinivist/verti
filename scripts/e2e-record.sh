#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mode="${MODE:-}"
name="${NAME:-}"
force="${FORCE:-0}"

usage() {
  printf 'usage: make e2e-record MODE=<repo|no-repo> NAME=<scenario> [FORCE=1]\n' >&2
}

fail() {
  printf '%s\n' "$1" >&2
  exit 1
}

if [ -z "$mode" ] || [ -z "$name" ]; then
  usage
  exit 1
fi

case "$mode" in
  repo|no-repo)
    ;;
  *)
    fail "invalid MODE: use repo or no-repo"
    ;;
esac

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

keys_path="$repo_root/e2e/tests/$mode/$name.keys"
golden_path="$repo_root/e2e/tests/$mode/$name.golden.out"
artifact_path="$repo_root/e2e/tests/artifacts/$mode/$name.out"

if [ "$force" != "1" ]; then
  for path in "$keys_path" "$golden_path" "$artifact_path"; do
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

mkdir -p "$home_dir" "$(dirname "$keys_path")" "$(dirname "$artifact_path")"

printf '== build verti ==\n' >&2
(
  cd "$repo_root"
  go build -o "$bin_path" ./cmd/verti
)

if [ "$mode" = "repo" ]; then
  printf '== setup test repo ==\n' >&2
  HOME="$home_dir" \
  TEST_REPO_DIR="$repo_dir" \
  VERTI_BIN="$bin_path" \
    "$repo_root/scripts/test-repo.sh" >&2
fi

printf '== record %s/%s ==\n' "$mode" "$name" >&2
if [ "$mode" = "repo" ]; then
  printf 'Recording into a shell rooted at %s\n' "$repo_dir" >&2
else
  printf 'Recording into a shell rooted at %s\n' "$home_dir" >&2
fi
printf 'Run the scenario, then press Ctrl+D to finish recording.\n' >&2

record_script() {
  home="$1"
  repo="$2"
  shift 2

  if [ "$mode" = "repo" ]; then
    HOME="$home" \
    HISTFILE=/dev/null \
    PATH="$tmpdir:$PATH" \
    TERM=xterm-256color \
    E2E_START_IN=repo \
    E2E_TEST_REPO="$repo" \
      script -q -e "$@" -c "$repo_root/scripts/e2e-shell.sh"
    return
  fi

  HOME="$home" \
  HISTFILE=/dev/null \
  PATH="$tmpdir:$PATH" \
  TERM=xterm-256color \
  E2E_START_IN=no-repo \
    script -q -e "$@" -c "$repo_root/scripts/e2e-shell.sh"
}

record_script "$home_dir" "$repo_dir" -E auto -I "$raw_in" -O "$raw_out"

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

printf '== replay %s/%s ==\n' "$mode" "$name" >&2
mkdir -p "$replay_home_dir"
if [ "$mode" = "repo" ]; then
  printf '== setup replay repo ==\n' >&2
  HOME="$replay_home_dir" \
  TEST_REPO_DIR="$replay_repo_dir" \
  VERTI_BIN="$bin_path" \
    "$repo_root/scripts/test-repo.sh" >&2
fi

record_script "$replay_home_dir" "$replay_repo_dir" -E never -O "$replay_raw_out" < "$clean_in"

tail -n +2 "$replay_raw_out" | sed '$d' > "$clean_out"

cp "$clean_in" "$keys_path"
cp "$clean_out" "$golden_path"
cp "$clean_out" "$artifact_path"

printf 'wrote %s\n' "$keys_path" >&2
printf 'wrote %s\n' "$golden_path" >&2
printf 'wrote %s\n' "$artifact_path" >&2
