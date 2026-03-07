#!/usr/bin/env sh
set -eu

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
test_repo="$repo_root/test-repo"

rm -rf "$test_repo"
mkdir -p "$test_repo"

git -C "$test_repo" init -b main
git -C "$test_repo" config user.name "Verti Test"
git -C "$test_repo" config user.email "verti-test@example.com"

printf "# test repo\n" > "$test_repo/README.md"
printf "baseline\n" > "$test_repo/test.md"
git -C "$test_repo" add README.md test.md
git -C "$test_repo" commit -m "chore: initial files"

printf "main update 1\n" >> "$test_repo/test.md"
git -C "$test_repo" add test.md
git -C "$test_repo" commit -m "feat: main update 1"

printf "main update 2\n" >> "$test_repo/test.md"
git -C "$test_repo" add test.md
git -C "$test_repo" commit -m "feat: main update 2"

git -C "$test_repo" checkout -b feature
printf "feature work\n" > "$test_repo/feature.txt"
git -C "$test_repo" add feature.txt
git -C "$test_repo" commit -m "feat: feature branch work"

git -C "$test_repo" checkout main
printf "main side change\n" > "$test_repo/main-only.txt"
git -C "$test_repo" add main-only.txt
git -C "$test_repo" commit -m "feat: main side change"

git -C "$test_repo" merge --no-ff feature -m "merge: feature into main"
