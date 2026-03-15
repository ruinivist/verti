#!/usr/bin/env sh
set -eu

start_in="${E2E_START_IN:?missing E2E_START_IN}"

export PS1='$ '
export HISTFILE=/dev/null
export TERM="${TERM:-xterm-256color}"

case "$start_in" in
  repo)
    repo_dir="${E2E_TEST_REPO:?missing E2E_TEST_REPO}"
    cd "$repo_dir"
    ;;
  no-repo)
    cd "${HOME:?missing HOME}"
    ;;
  *)
    printf 'unsupported E2E_START_IN: %s\n' "$start_in" >&2
    exit 1
    ;;
esac

exec bash --noprofile --norc -i
