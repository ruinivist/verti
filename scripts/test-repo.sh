#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_repo="$repo_root/test-repo"
verti_bin="${VERTI_BIN:-$repo_root/build/verti}"

rm -rf "$test_repo"
mkdir -p "$test_repo"

printf "\n== setup test repo ==\n"
git -C "$test_repo" init -b main
printf "test.md\n" >> "$test_repo/.git/info/exclude"
git -C "$test_repo" config user.name "Verti Test"
git -C "$test_repo" config user.email "verti-test@example.com"

printf "# test repo\n" > "$test_repo/README.md"
printf "baseline\n" > "$test_repo/test.md"
git -C "$test_repo" add README.md
git -C "$test_repo" commit -m "chore: initial files"

printf "\n== init verti ==\n"
(cd "$test_repo" && "$verti_bin" init)

printf "\n== build main history ==\n"
printf "main update 1\n" >> "$test_repo/README.md"
printf "main update 1\n" >> "$test_repo/test.md"
git -C "$test_repo" add README.md
git -C "$test_repo" commit -m "feat: main update 1"
feature2_base=$(git -C "$test_repo" rev-parse HEAD)

printf "main update 2\n" >> "$test_repo/README.md"
printf "main update 2\n" >> "$test_repo/test.md"
git -C "$test_repo" add README.md
git -C "$test_repo" commit -m "feat: main update 2"

printf "\n== create feature branch ==\n"
git -C "$test_repo" checkout -b feature
printf "feature update\n" >> "$test_repo/README.md"
printf "feature update\n" >> "$test_repo/test.md"
git -C "$test_repo" add README.md
git -C "$test_repo" commit -m "feat: feature branch work"

printf "\n== create feature2 branch ==\n"
git -C "$test_repo" checkout -b feature2 "$feature2_base"
printf "feature2 update\n" >> "$test_repo/README.md"
printf "feature2 update\n" >> "$test_repo/test.md"
git -C "$test_repo" add README.md
git -C "$test_repo" commit -m "feat: feature2 branch work"

printf "\n== return to main ==\n"
git -C "$test_repo" checkout main
