#!/usr/bin/env bash
#
# Generate the Krew plugin manifest from a published release.
#
# Krew needs the archive URL and the sha256 of every platform. Copying five
# checksums by hand, once per release, is a transcription error waiting to
# happen — and one wrong character means an install that fails for whoever
# runs it, not for you.
#
# Usage:
#   hack/krew-manifest.sh              # the latest release
#   hack/krew-manifest.sh v0.2.0       # a specific one
#   hack/krew-manifest.sh v0.2.0 --write   # write it to krew/why.yaml

set -euo pipefail

REPO="${KREW_REPO:-xavimf87/kubewhy}"
TAG="${1:-}"
WRITE=false
[[ "${2:-}" == "--write" || "${1:-}" == "--write" ]] && WRITE=true
[[ "$TAG" == "--write" ]] && TAG=""

command -v gh >/dev/null || { echo "the GitHub CLI (gh) is required" >&2; exit 2; }

if [[ -z "$TAG" ]]; then
  TAG="$(gh release view --repo "$REPO" --json tagName --jq .tagName)"
fi
VERSION="${TAG#v}"
echo "release: $TAG" >&2

CHECKSUMS="$(curl -fsSL "https://github.com/$REPO/releases/download/$TAG/checksums.txt")"

sha() {
  local file="$1" value
  value="$(awk -v f="$file" '$2 == f {print $1}' <<<"$CHECKSUMS")"
  if [[ -z "$value" ]]; then
    echo "no checksum for $file in the release" >&2
    exit 1
  fi
  printf '%s' "$value"
}

# platform <os> <arch> <archive extension> <binary name>
platform() {
  local os="$1" arch="$2" ext="$3" bin="$4"
  local file="kubewhy_${TAG}_${os}_${arch}.${ext}"
  cat <<EOF
    - selector:
        matchLabels:
          os: $os
          arch: $arch
      uri: https://github.com/$REPO/releases/download/$TAG/$file
      sha256: "$(sha "$file")"
      bin: $bin
      files:
        - from: $bin
          to: .
        - from: LICENSE
          to: .
EOF
}

manifest() {
  cat <<EOF
apiVersion: krew.googlecontainertools.github.com/v1alpha2
kind: Plugin
metadata:
  name: why
spec:
  version: $TAG
  homepage: https://github.com/$REPO
  shortDescription: Explain why a Kubernetes resource is not working
  description: |
    KubeWhy correlates the status, conditions, events and related objects of a
    Kubernetes resource and explains what they say about it, instead of leaving
    you to join kubectl get, describe and get events in your head.

      kubectl why pod api-7b89d8c9-xfd2
      kubectl why service payments
      kubectl why statefulset postgres

    It follows relationships. Ask about a Service and the Pods behind it are
    diagnosed too, with identical failures collapsed into one finding. Ask about
    an Ingress and it walks Ingress to Service to port to EndpointSlice to Pods.
    Ask about a StatefulSet and it names the replica holding up the ones after
    it, and which of them were never created at all.

    Every finding carries a stable identifier, a severity and a confidence, and
    KubeWhy says when the evidence does not identify a cause rather than
    inventing one. Machine-readable output is available with -o json, and the
    exit code says whether anything was found.

    It is read-only by design: it never modifies, restarts or deletes anything,
    never runs commands inside containers, and never reads Secret contents. It
    uses your existing kubeconfig, needs no cluster-admin, installs nothing into
    the cluster, and makes no network request other than to your API server.
  caveats: |
    Diagnoses Pods, Services, Deployments, StatefulSets, Ingresses and
    PersistentVolumeClaims.

    Run 'kubectl why rules' to see every diagnosis it can produce.
  platforms:
$(platform linux  amd64 tar.gz kubectl-why)
$(platform linux  arm64 tar.gz kubectl-why)
$(platform darwin amd64 tar.gz kubectl-why)
$(platform darwin arm64 tar.gz kubectl-why)
$(platform windows amd64 zip   kubectl-why.exe)
EOF
}

if $WRITE; then
  manifest > krew/why.yaml
  echo "written to krew/why.yaml" >&2
else
  manifest
fi
