#!/usr/bin/env bash
#
# Tests for hack/next-version.sh. Every release the project ever cuts goes
# through that script, so its arithmetic is worth checking.
#
# Usage: hack/next-version.test.sh

set -euo pipefail

SCRIPT="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/next-version.sh"
failures=0

# scenario <name> <tag or ""> <expected> <commit subject>...
scenario() {
  local name="$1" tag="$2" want="$3"; shift 3

  local dir; dir="$(mktemp -d)"
  trap 'rm -rf "$dir"' RETURN

  git -C "$dir" init -q -b main
  git -C "$dir" config user.email t@example.com
  git -C "$dir" config user.name Test
  git -C "$dir" commit -q --allow-empty -m "chore: initial"
  [[ -n "$tag" ]] && git -C "$dir" tag "$tag"

  local subject
  for subject in "$@"; do
    git -C "$dir" commit -q --allow-empty -m "$subject"
  done

  local got
  got="$(cd "$dir" && "$SCRIPT" 2>/dev/null || true)"

  if [[ "$got" == "$want" ]]; then
    printf '\033[32mok\033[0m   %s → %s\n' "$name" "${got:-(no release)}"
  else
    printf '\033[31mFAIL\033[0m %s: got "%s", want "%s"\n' "$name" "$got" "$want"
    failures=$((failures + 1))
  fi
}

scenario "first release"            ""       "v0.1.0"  "feat: anything"
scenario "nothing releasable"       "v0.1.0" ""        "docs: tidy" "chore: bump" "test: add"
scenario "a fix"                    "v0.1.0" "v0.1.1"  "fix: wrong diagnosis"
scenario "a performance change"     "v0.1.0" "v0.1.1"  "perf: fewer API calls"
scenario "a feature"                "v0.1.0" "v0.2.0"  "feat: new rule"
scenario "a feature outranks fixes" "v0.1.0" "v0.2.0"  "fix: a" "feat: b" "fix: c"
scenario "scopes are accepted"      "v0.1.0" "v0.2.0"  "feat(service): follow endpoints"
scenario "breaking before 1.0"      "v0.3.2" "v0.4.0"  "feat!: rename an identifier"
scenario "breaking in the body"     "v0.3.2" "v0.4.0"  $'fix: adjust\n\nBREAKING CHANGE: exit codes moved'
scenario "breaking after 1.0"       "v1.4.2" "v2.0.0"  "feat!: rename an identifier"
scenario "feature after 1.0"        "v1.4.2" "v1.5.0"  "feat: new rule"
scenario "fix after 1.0"            "v1.4.2" "v1.4.3"  "fix: a bug"
scenario "a revert is a patch"      "v1.4.2" "v1.4.3"  "revert: undo the thing"
scenario "unconventional is ignored" "v1.4.2" ""       "Add something the old way"
scenario "a feature buried in noise" "v1.4.2" "v1.5.0" "docs: a" "chore: b" "feat: c" "docs: d" "chore: e"
scenario "multi-line bodies parse"  "v1.4.2" "v1.4.3"  $'fix: one\n\nA body with\nseveral lines.' "docs: two"

echo
if (( failures )); then
  echo "$failures test(s) failed"
  exit 1
fi
echo "all version calculations correct"
