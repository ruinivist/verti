#!/usr/bin/env sh
set -eu

repo_dir="${E2E_TEST_REPO:?missing E2E_TEST_REPO}"

export PS1='$ '
export HISTFILE=/dev/null
export TERM="${TERM:-xterm-256color}"

cd "$repo_dir"
exec bash --noprofile --norc -i
