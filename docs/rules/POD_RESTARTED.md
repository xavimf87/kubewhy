# POD_RESTARTED

**Severity:** info
**Confidence:** certain
**Applies to:** Pod

## What it detects

A container that is running now but has restarted before. It answers the question `kubectl get pod` leaves you with:

```text
NAME                  READY   STATUS    RESTARTS      AGE
storage-provisioner   1/1     Running   6 (60m ago)   332d
```

Six restarts, and nothing anywhere that says why. The answer is in the Pod object, and today it takes `kubectl get pod -o yaml` and a scroll to find it.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `containerStatus` | `restartCount` | How many times. |
| `containerStatus` | `lastState.terminated.reason`, `.exitCode` | How the previous run ended. |
| `containerStatus` | `lastState.terminated.startedAt` → `.finishedAt` | How long that run lasted, which separates a container that died after a month from one that died after thirty seconds. |
| `containerStatus` | `state.running.startedAt` | How long it has been up since, which says whether it settled down. |

## Saying what Kubernetes does not keep

This is the important part. Kubernetes records **only the most recent termination**. Whatever happened at the other five restarts is not in the API and cannot be recovered from it, so the finding says so rather than implying that one termination explains them all.

```text
OBSERVATION
  Container "storage-provisioner" has restarted 6 times, most recently 7h46m ago

  The container is running now, so this is history rather than a problem.
  Kubernetes records only the most recent termination: the run before this one
  exited with code 1 and reason "Error" after running for 31s. What happened at
  the other restarts is not in the API.

  Suggested action
    The previous instance's logs are still available and are the only record of
    why it stopped.
      kubectl logs storage-provisioner -n kube-system -c storage-provisioner --previous
```

## Why it is informational

The container is working. An informational finding does not change the verdict, so the Pod still reads `✓ appears healthy` — it simply stops being silent about a number the user can already see and cannot explain.

The restart count in the report's `Status` section carries the same fact in one line: `Restarts  6  last one 7h46m ago`.

## Deliberate silences

- A container that has never restarted produces nothing.
- A container that is **still** cycling belongs to [`POD_CRASH_LOOP`](POD_CRASH_LOOP.md), which reports it as a failure rather than as history.
- A container whose last termination was an OOM kill belongs to [`POD_OOM_KILLED`](POD_OOM_KILLED.md).
- A container that is not running yet is not restart history; whatever is blocking it is the finding.

## Limitations

- One termination is all there is. A container that restarted for six different reasons looks exactly like one that restarted six times for the same reason.
- Events would say more, but they expire after about an hour, so for anything older this is the whole record.
