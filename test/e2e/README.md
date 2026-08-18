# End-to-end scenarios

`run.sh` applies [`examples/broken/`](../../examples/broken/) to a throwaway cluster and checks that KubeWhy produces the expected diagnosis identifier and exit code for each case.

These tests need a real cluster, so they are **not** part of `make test`. Unit tests cover the rules exhaustively without one; these exist to catch the gap between what we think Kubernetes reports and what it actually reports.

```bash
kind create cluster --name kubewhy-e2e
make build
make test-e2e
```

The script creates one namespace, applies the manifests, waits for the cluster to settle, runs the checks and deletes the namespace again. Set `KUBEWHY_E2E_YES=1` to skip the confirmation prompt, and `KUBEWHY_E2E_SETTLE` to change how long it waits (default 60 seconds — CrashLoopBackOff, image pull backoff and readiness probe failures all take time to appear).

In CI they run on `main`, on demand, and weekly, so a change in Kubernetes behaviour surfaces without slowing down every pull request.
