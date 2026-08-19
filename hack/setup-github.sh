#!/usr/bin/env bash
#
# One-time GitHub repository setup.
#
# Everything here could be clicked through the web interface. It is a script so
# that the settings are reviewable, repeatable, and the same on a fork.
#
# Usage:
#   hack/setup-github.sh                          # show what would change
#   hack/setup-github.sh --yes                    # apply it
#   hack/setup-github.sh --yes --protect          # also protect the main branch
#   hack/setup-github.sh --repo owner/name --yes  # before a remote is set up
#
# Branch protection is opt-in because it needs the CI checks to have run at
# least once for their names to exist.

set -euo pipefail

APPLY=false
PROTECT=false
REPO=""
while (( $# )); do
  case "$1" in
    --yes) APPLY=true ;;
    --protect) PROTECT=true ;;
    --repo) REPO="${2:-}"; shift ;;
    *) echo "unknown option: $1" >&2; exit 2 ;;
  esac
  shift
done

command -v gh >/dev/null || { echo "the GitHub CLI (gh) is required: https://cli.github.com" >&2; exit 2; }
gh auth status >/dev/null 2>&1 || { echo "run 'gh auth login' first" >&2; exit 2; }

if [[ -z "$REPO" ]]; then
  REPO="$(gh repo view --json nameWithOwner --jq .nameWithOwner 2>/dev/null || true)"
fi
if [[ -z "$REPO" ]]; then
  cat >&2 <<'MSG'
No repository found.

This directory has no GitHub remote yet. Either create the repository first:

  gh repo create kubewhy --public --source=. --remote=origin --push

or name it explicitly:

  hack/setup-github.sh --repo owner/kubewhy
MSG
  exit 2
fi

DESCRIPTION="🔍 A read-only kubectl plugin that explains why Kubernetes resources aren't working."
TOPICS=(kubernetes kubectl kubectl-plugin devops sre troubleshooting k8s golang cloud-native observability)

run() {
  if $APPLY; then
    "$@"
  else
    printf '  would run: %s\n' "$*"
  fi
}

echo "Repository: $REPO"
$APPLY || echo "(dry run — pass --yes to apply)"
echo

echo "Description, homepage and topics"
run gh repo edit "$REPO" \
  --description "$DESCRIPTION" \
  --homepage "https://github.com/$REPO" \
  --add-topic "$(IFS=,; echo "${TOPICS[*]}")"
echo

echo "Features"
run gh repo edit "$REPO" \
  --enable-issues --enable-discussions \
  --enable-wiki=false --enable-projects=false \
  --enable-squash-merge --enable-merge-commit=false --enable-rebase-merge=false \
  --delete-branch-on-merge
echo "  squash merging only, and the branch deleted after: the pull request title"
echo "  becomes the commit on main, and that title is what decides the version."
echo

# The issue templates apply these labels. A label that does not exist is
# dropped silently, so the templates only work once these are created.
label() {
  local name="$1" colour="$2" description="$3"
  if $APPLY; then
    gh label create "$name" --repo "$REPO" --color "$colour" --description "$description" --force >/dev/null
    printf '  %s\n' "$name"
  else
    printf '  would create label: %-18s %s\n' "$name" "$description"
  fi
}

echo "Labels"
label bug            d73a4a "KubeWhy crashed or behaved incorrectly as a program"
label diagnosis      b60205 "A finding that was wrong, misleading or unhelpful"
label rule           0e8a16 "A new or improved diagnostic rule"
label documentation  0075ca "Documentation only"
label enhancement    a2eeef "A new capability that is not a rule"
label performance    fbca04 "Fewer API calls, or a faster analysis"
label security       ee0701 "Anything touching cluster access or credentials"
label dependencies   0366d6 "Dependency updates"
label ci             ededed "Workflows and build tooling"
label "good first issue" 7057ff "A well-bounded place to start"
label "help wanted"  008672 "Extra attention is welcome here"
echo

if $PROTECT; then
  echo "Branch protection on main"
  if $APPLY; then
    gh api -X PUT "repos/$REPO/branches/main/protection" \
      --input - <<'JSON' >/dev/null
{
  "required_status_checks": {
    "strict": true,
    "contexts": [
      "Title",
      "Verify / Test (ubuntu-latest)",
      "Verify / Test (macos-latest)",
      "Verify / Test (windows-latest)",
      "Verify / Format",
      "Verify / Lint",
      "Verify / Scripts",
      "Verify / Release build",
      "End to end / Scenarios on kind"
    ]
  },
  "enforce_admins": false,
  "required_pull_request_reviews": {
    "required_approving_review_count": 0,
    "dismiss_stale_reviews": true
  },
  "restrictions": null,
  "allow_force_pushes": false,
  "allow_deletions": false,
  "required_linear_history": true
}
JSON
    echo "  main now requires the CI checks to pass"
  else
    echo "  would require the CI checks to pass before merging into main"
  fi
  echo
fi

cat <<'MSG'
Left to do by hand, because the API does not expose them well:

  * Social preview image
      Settings → General → Social preview → upload docs/assets/social-preview.png

  * Discussions categories
      Trim the defaults to Q&A and Ideas; the issue templates link to them.
MSG
