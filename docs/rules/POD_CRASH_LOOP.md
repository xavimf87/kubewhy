# POD_CRASH_LOOP

**Also emits:** `POD_INIT_CONTAINER_FAILED`
**Severity:** critical
**Confidence:** certain
**Applies to:** Pod

## What it detects

A container that keeps dying and being restarted. That shows up in two ways, and the rule looks for both:

1. **In backoff** — `state.waiting.reason` is `CrashLoopBackOff`, the state most `kubectl get` output catches.
2. **Between attempts** — the container is *running right now*, but has restarted at least twice and its previous run ended badly within the last ten minutes.

The second case matters more than it looks. A container that runs for two seconds and dies spends much of its time running, so a check that only looks for `CrashLoopBackOff` misses it exactly when the container happens to be up — and the failure would then be reported as "not ready, cause unknown". This was found by running the rules against a real cluster rather than against fixtures.

When the container is an init container, the finding is reported as `POD_INIT_CONTAINER_FAILED`, because the Pod cannot start at all until it succeeds.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `containerStatus` | `state.waiting.reason` | `CrashLoopBackOff`, when it is in backoff. |
| `containerStatus` | `lastState.terminated.exitCode`, `.reason`, `.signal`, `.message` | How the previous run ended. |
| `containerStatus` | `lastState.terminated.finishedAt` | How recently, which is what separates a loop from old history. |
| `containerStatus` | `restartCount` | How many times it has happened. |

The between-attempts case requires **all** of: at least two restarts, a previous run that exited non-zero, and that run having finished within the last ten minutes. Each condition removes a false positive — one restart is not a pattern, a clean exit is not a failure, and a container that failed last week and has run since is not failing now.

## Why the confidence is what it is

`certain` for the crash loop itself: Kubernetes states it. The reading of the exit code is separate, and the parts that are interpretation are listed under **Possible causes** rather than asserted:

| Exit code | What the rule says |
| --- | --- |
| `0` | The process finished successfully and the restart policy started it again — the workload may belong in a Job. |
| `126` | The command was found but could not be executed. |
| `127` | The command was not found in the image. |
| `137` | SIGKILL, sender not identified. Possible causes include an unrecorded OOM kill and a failing liveness probe. |
| `139` | Segmentation fault. |
| `143` | SIGTERM. |
| anything else | Kubernetes does not record why the process chose that code; the application's logs are the next step. |

When no previous termination is recorded, the rule says exactly that instead of guessing.

## Example

```text
ROOT CAUSE
  Container "worker" is restarting repeatedly

  Kubernetes restarted "worker" after each exit and is now backing off between
  attempts. The last run exited with code 1 and reason "Error". Kubernetes does
  not record why the process chose that exit code, so the application's own
  logs are the next place to look.

  Suggested action
    Read the logs of the previous container instance: the process itself
    reports why it exited, and Kubernetes does not record that.

      kubectl logs worker-8f6d9 -n prod -c worker --previous
```

## Limitations

- A container whose last termination was an OOM kill is reported by [`POD_OOM_KILLED`](POD_OOM_KILLED.md) instead, so one failure never produces two findings.
- KubeWhy does not read logs, so it cannot say what the application was doing. It gives you the exact command that will.
- Exit codes are a convention, not a Kubernetes guarantee; images are free to use their own.
