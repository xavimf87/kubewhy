# Architecture

KubeWhy is a CLI. The architecture exists to keep three things true:

1. Rules never talk to the API server, so they are deterministic and testable without a cluster.
2. Renderers never interpret evidence, so text and JSON always say the same thing.
3. Collectors decide what is worth querying, so a healthy resource costs two API calls.

## The pipeline

```text
cmd/kubectl-why
  └─ internal/cli          flags, exit codes, error messages
       └─ internal/kube    kubeconfig, typed client, kind resolution, error classification
            └─ internal/collect   API calls → snapshot
                 └─ internal/snapshot   normalized, read-only view
                      └─ internal/diagnosis/rules/<kind>   snapshot → []Diagnosis
                           └─ internal/diagnosis   prioritisation, report model
                                └─ internal/analyze   report assembly
                                     └─ internal/output   text and JSON renderers
```

Each arrow is a one-way dependency. Nothing below `collect` imports a Kubernetes client.

## The three concepts

The distinction between these is the product, not a detail of the implementation.

| Concept | Definition | Example |
| --- | --- | --- |
| **Evidence** | A fact read from the Kubernetes API, with the field it came from. Never inferred, never rephrased into a conclusion. | `lastState.terminated.reason = OOMKilled` |
| **Diagnosis** | An interpretation of evidence, with a stable identifier, a severity and a confidence. | `POD_OOM_KILLED`, critical, certain |
| **Suggestion** | Something a human may want to do next, with read-only commands. KubeWhy never runs them. | "Review the container's memory limit…" |

Confidence is not decoration:

- `certain` — Kubernetes states the cause. `lastState.terminated.reason == "OOMKilled"` is certain.
- `likely` — the evidence strongly supports the reading, but Kubernetes does not state it. Classifying a registry message as an authentication failure is likely.
- `possible` — the evidence is compatible with the reading and with others. Exit code 137 without an `OOMKilled` reason is possible, never certain.

When nothing reaches `possible`, the honest output is that no cause was identified. That is what `POD_NOT_READY` exists for.

## Snapshots

A snapshot is everything a diagnosis of one resource may need, collected once:

```text
snapshot.Pod
├── Pod
├── Events              normalized, deduplicated, most recent first
├── Owners              ReplicaSet → Deployment, StatefulSet, DaemonSet, Job → CronJob
├── Node                only when the Pod is scheduled and unhealthy
├── ConfigMaps/Secrets  existence only, only when a symptom points at configuration
├── PVCs                claim phase and storage class binding mode, only when relevant
├── Degradations        what could not be read, and why
└── Inspected           what was read, shown by --verbose
```

Two properties matter:

- **`Existence` has three values**: `Found`, `Missing`, `Unknown`. A read that was denied yields `Unknown`, and no rule may report `Unknown` as absent. Reporting a Secret as missing because RBAC blocked the check would be a wrong diagnosis of the worst kind.
- **`Now` is injected**, so every duration in the output is reproducible in tests.

## Rules

```go
type Rule[S any] interface {
    Meta() RuleMeta
    Evaluate(ctx context.Context, snap S) []Diagnosis
}
```

A rule is a pure function over a snapshot with metadata attached. `RuleMeta` declares the identifiers the rule can emit, which is what `kubectl why rules` prints and what keeps the documentation honest.

Rules are independent. Where two findings are related, a rule sets `CausedBy` on the consequence, and `diagnosis.Prioritize` orders root causes before their consequences:

```text
ROOT CAUSE    PersistentVolumeClaim "data" is Pending, not Bound
CONSEQUENCE   The scheduler found no node that can run this Pod
```

Prioritisation is: severity first, then root causes before consequences, then confidence. The sort is stable, so registration order breaks ties.

## Collectors

Collectors are where API cost is decided. The Pod collector always reads the Pod and its events, in parallel with the ownership chain, and then reads more **only when a symptom makes it relevant**:

| Extra read | Condition |
| --- | --- |
| Node | The Pod is scheduled and not fully ready. |
| ConfigMaps and Secrets | A container is in `CreateContainerConfigError`, `CreateContainerError` or `InvalidImageName`, or a mount failed. |
| PersistentVolumeClaims | The Pod is Pending, or a mount, attach or scheduling event failed. |
| StorageClass | A claim the Pod mounts has not bound. |

A healthy Pod therefore costs two requests, and no request is ever cluster-wide.

## Failure handling

| Situation | Behaviour |
| --- | --- |
| The requested resource does not exist | Typed error, exit code `3`, message that names the namespace and context. |
| The requested resource is forbidden | Typed error, exit code `4`. |
| A *related* object is forbidden | `Degradation` recorded, analysis continues, report says what is missing and why it mattered. |
| A related object does not exist | Usually evidence, not an error: it is often the answer. |
| The API times out | Exit code `2` with a message that points at `--timeout`. |

An analysis with degradations is never reported as healthy; its status is `unknown`.

## Adding a kind

1. Add the snapshot type in `internal/snapshot`.
2. Add the collector in `internal/collect`, querying only what a rule can use.
3. Add the rules package under `internal/diagnosis/rules/<kind>` with a `Rules()` and a `Catalog()`.
4. Add the report builder in `internal/analyze` and register the kind in `analyze.Analyze`.
5. Add the kind to `internal/kube.ResolveKind` and to `cli.ruleGroups`.

The renderers need no changes: they work off `Report`, `Section` and `Diagnosis`.
