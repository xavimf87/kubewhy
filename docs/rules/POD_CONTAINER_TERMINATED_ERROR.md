# POD_CONTAINER_TERMINATED_ERROR

**Also emits:** `POD_INIT_CONTAINER_FAILED`, `POD_EVICTED`
**Severity:** critical
**Confidence:** certain
**Applies to:** Pod

## What it detects

Two situations that a running Pod's rules would otherwise miss:

1. A container that exited with a non-zero code and **will not run again**, because the Pod has stopped or its restart policy is `Never`.
2. A Pod the kubelet **evicted**, which Kubernetes records on the Pod status rather than on any container.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `containerStatus` | `state.terminated.exitCode`, `.reason`, `.signal`, `.message` | How the container ended. |
| `podSpec` | `restartPolicy` | Why it will not be restarted. |
| `podStatus` | `status.reason` | `Evicted`. |
| `podStatus` | `status.message` | The node's own explanation, e.g. `The node was low on resource: memory.` |

## Why the confidence is what it is

`certain`: the exit code, the restart policy and the eviction reason are all facts. The reading of the exit code is shared with [`POD_CRASH_LOOP`](POD_CRASH_LOOP.md) and appears under possible causes, never as a conclusion.

## Deliberate silences

- A Pod in phase `Succeeded` produces **nothing**, whatever its containers did. A completed Job Pod is not a failure.
- A container that exited while the Pod is still alive and will be restarted produces nothing here; the backoff is what matters, and `POD_CRASH_LOOP` reports it.
- A container terminated with reason `OOMKilled` belongs to [`POD_OOM_KILLED`](POD_OOM_KILLED.md).

## Example

```text
ROOT CAUSE
  Container "import" exited with code 2

  Kubernetes reports that "import" terminated with a non-zero exit code and
  the Pod's restart policy is Never, so it will not run again.

  Evidence
    state.terminated.exitCode  2
    state.terminated.reason    Error
    restartPolicy              Never
```

## Limitations

- KubeWhy does not read logs, so it reports that the process failed, not what it was doing.
- Eviction is reported from the Pod object that remains; the replacement Pod, if any, is a different object and must be asked about separately.
