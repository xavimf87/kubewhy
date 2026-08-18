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
SETTLE_SECONDS="${KUBEWHY_E2E_SETTLE:-60}"

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

# scenario <kind> <name> <expected exit code> [expected diagnosis id]
scenario() {
  local kind="$1" name="$2" want_code="$3" want_id="${4:-}"
  local output code

  set +e
  output="$("$BINARY" "$kind" "$name" -n "$NAMESPACE" -o json 2>&1)"
  code=$?
  set -e

  if [[ "$code" != "$want_code" ]]; then
    fail "$kind/$name: exit code $code, want $want_code"
    info "$output"
    failures=$((failures + 1))
    return
  fi
  if [[ -n "$want_id" ]] && ! grep -q "\"$want_id\"" <<<"$output"; then
    fail "$kind/$name: expected diagnosis $want_id"
    info "$(grep -o '"id": "[A-Z_]*"' <<<"$output" | sort -u | tr '\n' ' ')"
    failures=$((failures + 1))
    return
  fi
  pass "$kind/$name${want_id:+ → $want_id}"
}

scenario pod healthy-demo             0
scenario pod oom-demo                 1 POD_OOM_KILLED
scenario pod crash-loop-demo          1 POD_CRASH_LOOP
scenario pod command-not-found-demo   1 POD_CRASH_LOOP
scenario pod image-pull-demo          1 POD_IMAGE_PULL_FAILED
scenario pod unschedulable-cpu-demo   1 POD_UNSCHEDULABLE_CPU
scenario pod node-selector-demo       1 POD_UNSCHEDULABLE_NODE_AFFINITY
scenario pod missing-configmap-demo   1 POD_MISSING_CONFIGMAP
scenario pod missing-secret-demo      1 POD_MISSING_SECRET
scenario pod readiness-probe-demo     1 POD_READINESS_PROBE_FAILED
scenario pod init-container-demo      1 POD_INIT_CONTAINER_FAILED
scenario pod pvc-demo                 1 POD_PVC_NOT_BOUND

scenario pvc kubewhy-data             1 PVC_STORAGECLASS_NOT_FOUND

scenario svc orphan-service-demo      1 SERVICE_NO_MATCHING_PODS
scenario svc payments-demo            1 SERVICE_NO_READY_ENDPOINTS

scenario deploy checkout-demo         1 POD_OOM_KILLED
scenario deploy payments-demo         1 DEPLOYMENT_UNAVAILABLE_REPLICAS

scenario ing api-demo                 1 INGRESS_SERVICE_NOT_FOUND

# A resource that does not exist has its own exit code.
scenario pod ghost-demo               3

echo
if (( failures > 0 )); then
  echo "$failures scenario(s) failed"
  exit 1
fi
echo "all scenarios passed"
