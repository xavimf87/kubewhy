# STATEFULSET_ORDERED_ROLLOUT_BLOCKED

**Severity:** critical
**Confidence:** certain
**Applies to:** StatefulSet

## What it detects

The lowest-ordinal Pod that is not ready, when the StatefulSet creates its Pods in order — and the replicas that were never created because of it.

This is the thing about StatefulSets that catches people out. With the default `podManagementPolicy: OrderedReady`, the controller creates one Pod at a time and waits for each to be ready before starting the next. So a single Pod that never becomes ready stops every Pod after it from existing at all:

```console
$ kubectl get statefulset postgres
NAME       READY   AGE
postgres   1/3     4h
```

Two replicas missing, and nothing anywhere says that only one of them is broken. `postgres-2` is not failing; it has never been created, and it will not be until `postgres-1` is ready.

## Evidence used

| Source | Field | Meaning |
| --- | --- | --- |
| `statefulSet` | `spec.podManagementPolicy` | `OrderedReady`. |
| `api` | `blockedBy` | The lowest-ordinal Pod that is not ready. |
| `api` | `neverCreated` | The replicas that do not exist, by name. |

Pods are sorted by **ordinal**, not by name. Lexicographically `postgres-10` comes before `postgres-2`, and every conclusion drawn from that order would be wrong.

## Example

```text
ROOT CAUSE
  postgres-1 is not ready, so the 1 replica after it has not been created

  This StatefulSet uses podManagementPolicy OrderedReady, so it creates one Pod
  at a time and waits for each to be ready before starting the next. Pod 1 is
  not ready, so nothing after it exists yet. Those replicas are not failing; the
  controller has not created them. Fixing this one Pod releases the rest.

  Suggested action
    Diagnose the blocking Pod; every replica behind it is waiting on that one
    answer.
      kubectl why pod postgres-1 -n prod
```

The report's `Replicas by ordinal` section shows the same thing as a list, including the replicas that do not exist:

```text
Replicas by ordinal
  postgres-0  Ready           1h old
  postgres-1  Pending         1h old
  postgres-2  does not exist  never created
```

## Deliberate silences

- `podManagementPolicy: Parallel` creates all the Pods at once, so nothing blocks anything. The rule does not fire, and the missing replicas are a different problem.
- A set that is scaled to zero, or one where every wanted replica exists, produces nothing.

## Limitations

- The rule reports **which** Pod blocks the rest. Why that Pod is not ready comes from the Pod rules, which run over the same Pods and appear alongside.
- A Pod name with no ordinal suffix cannot be placed in the order; such Pods sort by name and are not treated as blockers.
