#!/usr/bin/env bash
#
# End-to-end scenarios for KubeWhy.
#
# Applies the deliberately broken manifests to a throwaway cluster, waits for
# Kubernetes to report what it thinks, and checks that KubeWhy produces the
# expected diagnosis identifier and exit code.
#
# Usage:
#   test/e2e/run.sh                 # uses the current kubectl context
#   KUBEWHY_E2E_YES=1 test/e2e/run.sh
#
# The script creates one namespace and deletes it again. It changes nothing
# else, and KubeWhy itself never writes to the cluster.

set -euo pipefail

NAMESPACE="${KUBEWHY_E2E_NAMESPACE:-kubewhy-e2e}"
BINARY="${KUBEWHY_BINARY:-./bin/kubectl-why}"
EXAMPLES="examples/broken"
SETTLE_SECONDS="${KUBEWHY_E2E_SETTLE:-45}"

fail() { printf '\033[31mFAIL\033[0m %s\n' "$*"; }
pass() { printf '\033[32mok\033[0m   %s\n' "$*"; }
info() { printf '     %s\n' "$*"; }

failures=0

if [[ ! -x "$BINARY" ]]; then
  echo "binary not found at $BINARY; run 'make build' first" >&2
  exit 2
fi

context="$(kubectl config current-context)"
echo "KubeWhy end-to-end scenarios"
echo "  context   $context"
echo "  namespace $NAMESPACE (created and deleted by this script)"
echo

if [[ "${KUBEWHY_E2E_YES:-}" != "1" ]]; then
  read -r -p "Run against '$context'? [y/N] " answer
  [[ "$answer" == "y" || "$answer" == "Y" ]] || exit 0
fi

cleanup() {
  echo
  info "deleting namespace $NAMESPACE"
  kubectl delete namespace "$NAMESPACE" --wait=false >/dev/null 2>&1 || true
}
trap cleanup EXIT

kubectl create namespace "$NAMESPACE" >/dev/null
for manifest in "$EXAMPLES"/*.yaml; do
  kubectl apply -n "$NAMESPACE" -f "$manifest" >/dev/null
done

info "waiting ${SETTLE_SECONDS}s for the cluster to report"
sleep "$SETTLE_SECONDS"
echo

# scenario <pod> <expected exit code> [expected diagnosis id]
scenario() {
  local pod="$1" want_code="$2" want_id="${3:-}"
  local output code

  set +e
  output="$("$BINARY" pod "$pod" -n "$NAMESPACE" -o json 2>&1)"
  code=$?
  set -e

  if [[ "$code" != "$want_code" ]]; then
    fail "$pod: exit code $code, want $want_code"
    info "$output"
    failures=$((failures + 1))
    return
  fi
  if [[ -n "$want_id" ]] && ! grep -q "\"$want_id\"" <<<"$output"; then
    fail "$pod: expected diagnosis $want_id"
    info "$(grep -o '"id": "[A-Z_]*"' <<<"$output" | sort -u | tr '\n' ' ')"
    failures=$((failures + 1))
    return
  fi
  pass "$pod${want_id:+ → $want_id}"
}

scenario healthy-demo             0
scenario oom-demo                 1 POD_OOM_KILLED
scenario crash-loop-demo          1 POD_CRASH_LOOP
scenario command-not-found-demo   1 POD_CRASH_LOOP
scenario image-pull-demo          1 POD_IMAGE_PULL_FAILED
scenario unschedulable-cpu-demo   1 POD_UNSCHEDULABLE_CPU
scenario node-selector-demo       1 POD_UNSCHEDULABLE_NODE_AFFINITY
scenario missing-configmap-demo   1 POD_MISSING_CONFIGMAP
scenario missing-secret-demo      1 POD_MISSING_SECRET
scenario readiness-probe-demo     1 POD_READINESS_PROBE_FAILED
scenario init-container-demo      1 POD_INIT_CONTAINER_FAILED
scenario pvc-demo                 1 POD_PVC_NOT_BOUND

# A resource that does not exist has its own exit code.
scenario ghost-demo               3

echo
if (( failures > 0 )); then
  echo "$failures scenario(s) failed"
  exit 1
fi
echo "all scenarios passed"
