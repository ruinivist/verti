#!/usr/bin/env sh
set -eu

start_in="${E2E_START_IN:?missing E2E_START_IN}"

export PS1='$ '
export HISTFILE=/dev/null
export TERM="${TERM:-xterm-256color}"

configure_git_identity() {
  git config --global user.name "Verti Test"
  git config --global user.email "verti-test@example.com"
}

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

configure_git_identity

exec bash --noprofile --norc -i
