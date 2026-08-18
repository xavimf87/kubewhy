#!/usr/bin/env bash
#
# Work out the version that the commits since the last release deserve.
#
# Prints the next tag (v1.2.3) on stdout, or nothing at all when nothing
# releasable has landed. The reasoning goes to stderr so it can be read in a
# workflow log without polluting the output.
#
# Usage:
#   hack/next-version.sh            # what would the next release be?
#   hack/next-version.sh --explain  # same, with the commits that decided it

set -euo pipefail

EXPLAIN=false
[[ "${1:-}" == "--explain" ]] && EXPLAIN=true

log() { printf '%s\n' "$*" >&2; }

last_tag="$(git tag --list 'v[0-9]*' --sort=-v:refname | head -1)"

if [[ -z "$last_tag" ]]; then
  # Nothing has ever been released. The first release is 0.1.0: the history
  # before the first tag predates any commit convention, so deriving a version
  # from it would be reading tea leaves.
  log "no previous release; the first one is v0.1.0"
  echo "v0.1.0"
  exit 0
fi

current="${last_tag#v}"
IFS=. read -r major minor patch <<<"$current"
log "last release: $last_tag"

# Conventional Commits, as far as versioning cares about them.
type_re='^[a-z]+(\([^)]*\))?'
breaking_re="${type_re}!:"
feature_re='^feat(\([^)]*\))?:'
patch_re='^(fix|perf|revert)(\([^)]*\))?:'

bump=none
while IFS= read -r -d '' commit; do
  # git separates entries with a newline, so every record after the first
  # arrives with one in front of the subject.
  while [[ "$commit" == $'\n'* ]]; do commit="${commit#$'\n'}"; done

  subject="${commit%%$'\n'*}"
  if [[ -z "$subject" ]]; then
    continue
  fi

  if [[ "$subject" =~ $breaking_re ]] || [[ "$commit" == *"BREAKING CHANGE"* ]]; then
    bump=breaking
    if $EXPLAIN; then log "  breaking  $subject"; fi
    continue
  fi
  if [[ "$subject" =~ $feature_re ]]; then
    if [[ "$bump" != breaking ]]; then bump=feature; fi
    if $EXPLAIN; then log "  feature   $subject"; fi
    continue
  fi
  if [[ "$subject" =~ $patch_re ]]; then
    if [[ "$bump" == none ]]; then bump=patch; fi
    if $EXPLAIN; then log "  fix       $subject"; fi
    continue
  fi
  if $EXPLAIN; then log "  no bump   $subject"; fi
done < <(git log --format='%s%n%b%x00' "${last_tag}..HEAD")

case "$bump" in
  none)
    log "nothing releasable since $last_tag"
    exit 0
    ;;
  breaking)
    if (( major == 0 )); then
      # A 0.x release makes no stability promise, so a breaking change moves
      # the minor rather than declaring 1.0 on its own.
      minor=$((minor + 1)); patch=0
      log "breaking change before 1.0: minor bump"
    else
      major=$((major + 1)); minor=0; patch=0
      log "breaking change: major bump"
    fi
    ;;
  feature)
    minor=$((minor + 1)); patch=0
    log "new features: minor bump"
    ;;
  patch)
    patch=$((patch + 1))
    log "fixes only: patch bump"
    ;;
esac

echo "v${major}.${minor}.${patch}"
