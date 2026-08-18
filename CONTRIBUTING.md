# Contributing to KubeWhy

Thanks for being here. KubeWhy is deliberately built so that the most valuable contribution — **a new diagnosis rule** — is also one of the smallest and best-bounded tasks in the codebase.

## Local setup

You need Go 1.24 or newer. No cluster is required for the unit tests.

```bash
git clone https://github.com/xavimf87/kubewhy
cd kubewhy

make check     # gofmt, go vet, go test — what CI runs
make build     # ./bin/kubectl-why
make help      # every target
```

To try it against a cluster:

```bash
make install                       # installs kubectl-why into your GOBIN
kubectl why pod some-pod -n some-namespace
```

[`examples/broken/`](examples/broken/) has manifests that fail on purpose. Apply them to a throwaway cluster (kind, k3d, minikube) and point KubeWhy at them.

## The architecture in one minute

```text
CLI → resolver → collectors → snapshot → rules → diagnoses → renderers
```

Three invariants hold everywhere, and a pull request that breaks one will be asked to change:

1. **Rules never call the Kubernetes API.** Everything a rule needs is in the snapshot. That is what makes rules testable without a cluster.
2. **Renderers never interpret.** If text and JSON could disagree, the logic is in the wrong place.
3. **Evidence, diagnosis and suggestion stay separate.** Evidence is read from the API. A diagnosis interprets it and carries a confidence. A suggestion is for a human to act on.

[`docs/architecture.md`](docs/architecture.md) has the full picture.

## How to add a diagnosis rule

Say you want to detect that a Pod is stuck because of something the current rules miss.

### 1. Write the rule

Rules for Pods live in `internal/diagnosis/rules/pod/`. One file per rule, named after what it detects.

```go
func myRule() diagnosis.Rule[*snapshot.Pod] {
    return diagnosis.RuleFunc[*snapshot.Pod]{
        Metadata: diagnosis.RuleMeta{
            ID:          IDMyFinding,
            Title:       "Short human name",
            Description: "What it detects, and from which evidence.",
        },
        Fn: evaluateMyRule,
    }
}

func evaluateMyRule(_ context.Context, snap *snapshot.Pod) []diagnosis.Diagnosis {
    // Read snap. Return nothing when the evidence does not support a finding.
}
```

Add the identifier to the `const` block in `pod.go` and register the rule in `Rules()`.

### 2. Choose the severity and confidence honestly

| Confidence | Use when |
| --- | --- |
| `certain` | Kubernetes states it. `lastState.terminated.reason == "OOMKilled"` is certain. |
| `likely` | The evidence strongly supports your reading, but Kubernetes does not say it. Classifying a registry error message is likely. |
| `possible` | The evidence is compatible with your reading *and with others*. Exit code 137 without an `OOMKilled` reason is possible, never certain. |

If you cannot reach `possible`, the rule should return nothing and let the fallback report the observed state. **"I don't know" beats a wrong answer**, and this is the single most important rule in the project.

Put anything you cannot prove in `PossibleCauses`, never in `Summary` or `Explanation`.

### 3. Write the tests, including the negative one

Tests live next to the rules and use the builders in `internal/kubetest`:

```go
snap := kubetest.Snap(kubetest.Pod("api").
    Container(kubetest.Container("api").
        Waiting("CrashLoopBackOff", "back-off").
        LastTerminated("OOMKilled", 137)).
    Build())
```

Every rule needs at least:

- a **positive** case that produces the finding;
- a **negative** case that must not — the near-miss that would be a false positive;
- a **healthy** case, covered by the shared table in `pod_test.go`.

The negative case is the one reviewers will read first. `POD_OOM_KILLED` has "exit code 137 without an `OOMKilled` reason"; yours needs its equivalent.

### 4. Add a collector only if you must

If your rule needs data the snapshot does not carry, extend `internal/collect`. Two constraints:

- The extra API call must be **conditional on a symptom**, so healthy resources stay cheap.
- A denied read must record a `Degradation` and yield `Unknown` — never `Missing`. Reporting an object as absent because RBAC blocked the check is the worst kind of wrong answer.

### 5. Document the rule

Copy [`docs/rules/_TEMPLATE.md`](docs/rules/_TEMPLATE.md) to `docs/rules/YOUR_ID.md`, fill it in, and add a row to `docs/rules/README.md`. The **Limitations** section is not optional: a rule that cannot say where it stops cannot be trusted.

### Checklist

- [ ] Rule implemented and registered in `Rules()`
- [ ] Identifier added to the `const` block
- [ ] Positive, negative and healthy tests
- [ ] `docs/rules/<ID>.md` written, index updated
- [ ] `make check` passes

## Adding a resource kind

Bigger, but the same shape: snapshot type, collector, rules package, report builder in `internal/analyze`, kind registered in `internal/kube.ResolveKind` and `cli.ruleGroups`. See the "Adding a kind" section of [`docs/architecture.md`](docs/architecture.md). Open an issue first so we can agree on the scope.

## Output and golden files

The text renderer is covered by golden files in `internal/output/testdata/`. When you change the layout on purpose:

```bash
make golden
```

Review the diff. It is the product's user interface, and it should read like `kubectl`, `gh` or `terraform`: restrained, aligned, no dashboards.

## Pull requests

- Keep them focused. One rule, one fix, one refactor.
- Explain *why*, not just what. If the change affects a diagnosis, show the before and after output.
- Note whether you changed anything users automate against: CLI syntax, rule identifiers, JSON fields, exit codes. These are treated as public API even before 1.0.
- Write commits in the imperative mood: "Add POD_FAILED_MOUNT rule".
- CI runs `gofmt`, `go vet`, `go test` and a build on Linux, macOS and Windows.

## Things this project will not do

So you do not spend effort on a PR that cannot be merged:

- Anything that writes to a cluster, or suggests a mutating command by default.
- Reading Secret contents, or fetching container logs automatically.
- Telemetry, analytics or any outbound request other than to the API server.
- An LLM or embeddings layer. KubeWhy earns trust by being deterministic and explainable.
- Cloud-provider SDKs, or behaviour tied to one managed Kubernetes distribution.
- Namespace-wide or cluster-wide scanning, which is a different product.

Questions are welcome as issues. So are reports of a diagnosis that was wrong or unhelpful — those are the most valuable bug reports this project can get.
