# POD_CRASH_LOOP

**Also emits:** `POD_INIT_CONTAINER_FAILED`
**Severity:** critical
**Confidence:** certain
**Applies to:** Pod

## What it detects

A container in `CrashLoopBackOff`: the kubelet restarted it after each exit and is now waiting between attempts. When the container is an init container, the finding is reported as `POD_INIT_CONTAINER_FAILED`, because the Pod cannot start at all until it succeeds.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `containerStatus` | `state.waiting.reason` | `CrashLoopBackOff`. |
| `containerStatus` | `lastState.terminated.exitCode`, `.reason`, `.signal`, `.message` | How the previous run ended. |
| `containerStatus` | `restartCount` | How many times it has happened. |

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
