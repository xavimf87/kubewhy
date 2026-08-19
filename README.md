<p align="center">
  <img src="docs/assets/logo.svg" alt="" width="132">
</p>

<h1 align="center">KubeWhy</h1>

<p align="center">
  <strong>Stop describing Kubernetes problems. Start explaining them.</strong>
</p>

<p align="center">
  A read-only <code>kubectl</code> plugin that tells you <em>why</em> a Kubernetes resource is not working.
</p>

<p align="center">
  <a href="https://github.com/xavimf87/kubewhy/releases/latest"><img alt="Latest release" src="https://img.shields.io/github/v/release/xavimf87/kubewhy?sort=semver&color=4A8BF5"></a>
  <a href="https://github.com/xavimf87/kubewhy/actions/workflows/main.yml"><img alt="Main pipeline" src="https://github.com/xavimf87/kubewhy/actions/workflows/main.yml/badge.svg"></a>
  <a href="https://pkg.go.dev/github.com/xavimf87/kubewhy"><img alt="Go reference" src="https://pkg.go.dev/badge/github.com/xavimf87/kubewhy.svg"></a>
  <a href="go.mod"><img alt="Go version" src="https://img.shields.io/github/go-mod/go-version/xavimf87/kubewhy?color=4A8BF5"></a>
  <a href="LICENSE"><img alt="Apache 2.0 licence" src="https://img.shields.io/badge/licence-Apache%202.0-blue.svg"></a>
  <a href="docs/security.md"><img alt="Read-only by design" src="https://img.shields.io/badge/cluster%20access-read--only-brightgreen.svg"></a>
</p>

---

```console
$ kubectl why pod checkout-7c8cc8679-j9qd8 -n prod

✗ Pod/checkout-7c8cc8679-j9qd8 is unhealthy

ROOT CAUSE
  Container "checkout" was terminated for exceeding its memory limit

  Kubernetes reports that "checkout" was killed with reason OOMKilled. The
  kernel terminates a container when the memory it uses reaches the limit
  configured for it.

  Evidence
    state.waiting.reason           CrashLoopBackOff
      back-off 5m0s restarting failed container
    restartCount                   8
    lastState.terminated.exitCode  137
    lastState.terminated.reason    OOMKilled
    resources.requests.memory      256Mi
    resources.limits.memory        512Mi

  Possible causes
    • the application uses more memory than it was allocated
    • the workload's memory requirements grew beyond the configured limit
    • the configured memory limit is too low for this workload

  Suggested action
    Review the container's memory limit and the application's memory usage to
    decide which of the two should change.

      kubectl logs checkout-7c8cc8679-j9qd8 -n prod -c checkout --previous

Status
  Phase     Running
  Ready     0/1 containers
  Restarts  8
  Age       1h
  Node      node-3

Containers
  checkout  Waiting (CrashLoopBackOff)  8 restarts

Owned by
  Deployment/checkout
  └─ ReplicaSet/checkout-7c8cc8679
     └─ Pod/checkout-7c8cc8679-j9qd8
```

That is one command instead of `get`, `describe`, `get events`, `get rs`, `get deployment` and a mental join of all of them.

It follows relationships too. Ask about a Service, and it diagnoses the Pods behind it — collapsing identical failures instead of repeating them:

```console
$ kubectl why service payments -n prod

✗ Service/payments is unhealthy

ROOT CAUSE
  Container "api" is failing its readiness probe (3 of 3 Pods)

  A container that fails its readiness probe is removed from the endpoints of
  every Service that selects this Pod, so it receives no traffic. Kubernetes
  reports that the probe failed; the reason the application answered that way is
  in its own logs.

  Evidence
    reason                 Unhealthy (x37)
      Readiness probe failed: HTTP probe failed with statuscode: 503
    readinessProbe         http GET /healthz on port 8080
    readinessProbe.timing  delay 5s, period 10s, timeout 1s, failureThreshold 3

CONSEQUENCE
  The Service has no ready endpoints, so it accepts no traffic

Backends
  Matching Pods    3
  Ready endpoints  0 of 3  from EndpointSlice
```

The same walk happens for an Ingress (`Ingress → Service → port → EndpointSlice → Pods`), for a Deployment, and for a StatefulSet — where it also tells you which replica is holding up the ones after it, and which of them were never created at all. One rule set, applied wherever it is relevant.

---

## Why KubeWhy?

Kubernetes already tells you everything. It just tells you in six places at once: Pod status, container states, conditions, events, owner references, and the objects around them. Correlating them is a skill, and doing it at 3am under pressure is a chore.

KubeWhy does that correlation and states a conclusion — **or says plainly that the evidence does not support one**:

```text
Container "worker" is restarting repeatedly

Kubernetes restarted "worker" after each exit and is now backing off between
attempts. The last run exited with code 1 and reason "Error". Kubernetes does
not record why the process chose that exit code, so the application's own logs
are the next place to look.
```

It will never tell you that your application has a memory leak, because Kubernetes cannot know that.

**What KubeWhy is:** a deterministic troubleshooting CLI.
**What it is not:** a dashboard, an agent, an operator, an observability platform, or an AI assistant.

---

## Installation

KubeWhy is a single binary called `kubectl-why`. **Put it anywhere on your `PATH` and kubectl exposes it as `kubectl why`** — that is all a kubectl plugin is, and there is nothing to configure:

```console
$ kubectl plugin list
/usr/local/bin/kubectl-why

$ kubectl why pod api-7b89d8c9-xfd2
```

Running it directly as `kubectl-why pod api-7b89d8c9-xfd2` does exactly the same thing. Use whichever reads better to you; the rest of this README uses `kubectl why`.

### Download a binary

No Go toolchain needed. Pick your platform from the [latest release](https://github.com/xavimf87/kubewhy/releases/latest):

```bash
VERSION=$(curl -fsSL https://api.github.com/repos/xavimf87/kubewhy/releases/latest \
  | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p')
OS=$(uname -s | tr '[:upper:]' '[:lower:]')
ARCH=$(uname -m | sed 's/x86_64/amd64/; s/aarch64/arm64/')

curl -fsSL "https://github.com/xavimf87/kubewhy/releases/download/$VERSION/kubewhy_${VERSION}_${OS}_${ARCH}.tar.gz" | tar xz
sudo mv kubectl-why /usr/local/bin/

kubectl why --help          # it is a kubectl plugin now
```

The version is resolved rather than written in, so the command does not go stale between releases. Windows is a `.zip` of the same shape.

Verify the archive against `checksums.txt` from the same release if you like.

If you download through a browser instead, macOS quarantines the file and Gatekeeper refuses to run it; `xattr -d com.apple.quarantine /usr/local/bin/kubectl-why` clears that. Fetching it with `curl` as above never sets the attribute.

### With `go install`

**Requires Go 1.26 or newer.** That is not a choice KubeWhy makes: the Kubernetes client libraries it builds on require it, and an older toolchain fails with `invalid go version '1.26.0'` before it compiles anything. If you are not on 1.26, download a binary instead — it needs no Go at all.

```bash
go install github.com/xavimf87/kubewhy/cmd/kubectl-why@latest

kubectl why --help          # once your GOBIN, usually ~/go/bin, is on your PATH
```

### From source

```bash
git clone https://github.com/xavimf87/kubewhy
cd kubewhy
make build
sudo mv bin/kubectl-why /usr/local/bin/

kubectl why --help
```

### Krew

Krew is a package manager for kubectl plugins. It installs the same binary into a directory it puts on your `PATH`, so `kubectl why` works exactly as above — what it adds is upgrades and removal in one command:

```bash
kubectl krew install why      # not available yet
kubectl krew upgrade why
kubectl krew uninstall why
```

**KubeWhy is not in the Krew index yet.** The manifest under [`krew/`](krew/) is ready for the submission; until it is merged, use one of the methods above. This line will say so when that changes.

---

## Usage

```bash
kubectl why RESOURCE NAME [flags]
```

```bash
kubectl why pod api-7b89d8c9-xfd2
kubectl why pod api-7b89d8c9-xfd2 -n production
kubectl why pod api-7b89d8c9-xfd2 --context prod-cluster
kubectl why pod api-7b89d8c9-xfd2 -o json
kubectl why pod api-7b89d8c9-xfd2 --verbose
```

| Flag | Description |
| --- | --- |
| `-n`, `--namespace` | Namespace of the resource. Defaults to the current context's namespace. |
| `--context` | kubeconfig context to use. |
| `--kubeconfig` | Path to the kubeconfig file. |
| `-o`, `--output` | `text` (default) or `json`. |
| `--verbose` | Show every piece of evidence, the rule behind each finding, and the queries KubeWhy made. |
| `--color` | `auto` (default), `always` or `never`. See below. |
| `--no-color` | The familiar spelling of `--color never`. |
| `--timeout` | Maximum time to wait for the Kubernetes API. Default `15s`. |
| `-V`, `--version` | Print version information. |

### Colour

Output is coloured the way you would expect from any other CLI, and the rules are the ones the ecosystem already agreed on:

| Situation | Result |
| --- | --- |
| Writing to a terminal | Coloured |
| Piped or redirected | Plain, so logs and `grep` stay clean |
| `--color always` | Coloured anyway — for `less -R` and CI logs that render ANSI |
| `NO_COLOR` set ([no-color.org](https://no-color.org)) | Plain, unless you passed `--color always` |
| `CLICOLOR_FORCE` set | Coloured, even through a pipe |
| `TERM=dumb` | Plain |
| Windows console | Coloured, with virtual terminal processing enabled automatically; plain on consoles too old to support it |

Only the original sixteen ANSI colours are used, so KubeWhy renders correctly in every terminal, multiplexer and CI log viewer, and it follows your own colour theme instead of overriding it.

**Colour is never the only signal.** Severity is carried by the glyph as well as the colour (`✓`, `!`, `✗`, and `[ok]`, `[!]`, `[x]` where Unicode is unavailable), and stripping the escape sequences gives back exactly the plain output — there is a test that asserts it.

Two helper commands:

```bash
kubectl why rules      # every rule and the identifiers it can produce
kubectl why version
```

---

## Supported resources

| Resource | Aliases | What it correlates |
| --- | --- | --- |
| Pod | `pods`, `po` | Container states, conditions, events, ownership, node, mounted claims, referenced config |
| Service | `services`, `svc` | Selector, matching Pods, EndpointSlices, ports — and the Pod rules over its backends |
| Deployment | `deployments`, `deploy` | Replica counts, conditions, ReplicaSets — and the Pod rules over its Pods, aggregated |
| StatefulSet | `statefulsets`, `sts` | Ordered creation, per-replica volumes, the governing headless Service, update strategy — and the Pod rules over its replicas |
| Ingress | `ingresses`, `ing` | Ingress → Service → port → EndpointSlice → Pods, plus the ingress class |
| PersistentVolumeClaim | `persistentvolumeclaims`, `pvc` | Phase, storage class and binding mode, provisioning events, consuming Pods |

Quality over coverage: a resource appears here only when its diagnoses are worth trusting.

---

## Supported diagnoses

Every finding carries a **stable identifier**, a **severity** and a **confidence**. Run `kubectl why rules` for the live list.

| Identifier | What it detects |
| --- | --- |
| `POD_OOM_KILLED` | A container Kubernetes terminated with reason `OOMKilled`, with its memory request and limit. |
| `POD_CRASH_LOOP` | A container in `CrashLoopBackOff`, with what the last termination does and does not prove. |
| `POD_INIT_CONTAINER_FAILED` | An init container blocking the Pod from starting. |
| `POD_CONTAINER_TERMINATED_ERROR` | A container that exited non-zero and will not be restarted. |
| `POD_EVICTED` | A Pod the kubelet evicted, with the node's reason. |
| `POD_IMAGE_PULL_FAILED` | An image that cannot be pulled, classified as not-found, unauthorised, rate-limited or unreachable when the registry message says so. |
| `POD_UNSCHEDULABLE` | The scheduler rejected every node, with its reasons normalised. |
| `POD_UNSCHEDULABLE_CPU` / `_MEMORY` | Insufficient capacity was the only reason. |
| `POD_UNTOLERATED_TAINT` | Taints were the only reason. |
| `POD_UNSCHEDULABLE_NODE_AFFINITY` | Node selector or affinity was the only reason. |
| `POD_UNSCHEDULABLE_VOLUME` | Volume binding was the only reason. |
| `POD_SCHEDULING_GATED` | The Pod is held by scheduling gates and was never scheduled — on purpose. |
| `POD_READINESS_PROBE_FAILED` | A readiness probe failing, with the probe's target and timing. |
| `POD_LIVENESS_PROBE_FAILED` | A liveness probe failing, which explains restarts without a crash. |
| `POD_STARTUP_PROBE_FAILED` | A startup probe failing. |
| `POD_FAILED_MOUNT` | A volume the kubelet could not mount. |
| `POD_MISSING_CONFIGMAP` | A required ConfigMap the API server reports as absent. |
| `POD_MISSING_SECRET` | A required Secret the API server reports as absent (existence only — never contents). |
| `POD_CREATE_CONTAINER_CONFIG_ERROR` | A container that cannot be created from its configuration. |
| `POD_PVC_NOT_FOUND` | A claim the Pod mounts that does not exist. |
| `POD_PVC_NOT_BOUND` | A claim that has not bound — and, when it uses `WaitForFirstConsumer`, why that is expected rather than broken. |
| `POD_NODE_NOT_READY` | The Pod's node is not reporting Ready. |
| `POD_RESTARTED` | A working container that has restarted before: how many times, how long ago, and how the last run ended — with an explicit note that Kubernetes keeps only the most recent one. |
| `POD_NOT_READY` | The Pod is not ready and **no rule could explain it**. KubeWhy says so instead of inventing a cause. |

### Service

| Identifier | What it detects |
| --- | --- |
| `SERVICE_NO_MATCHING_PODS` | The selector matches no Pod in the namespace. |
| `SERVICE_NO_READY_ENDPOINTS` | Backends exist but none is receiving traffic — with the Pod findings that explain why. |
| `SERVICE_SOME_ENDPOINTS_NOT_READY` | Only part of the backends are serving. |
| `SERVICE_NO_ENDPOINTS_WITHOUT_SELECTOR` | A selector-less Service whose endpoints nothing has published. |
| `SERVICE_TARGET_PORT_NOT_FOUND` | A named `targetPort` that no backend Pod declares. |

### Deployment

| Identifier | What it detects |
| --- | --- |
| `DEPLOYMENT_UNAVAILABLE_REPLICAS` | Fewer available replicas than requested — with the Pod failures aggregated, not repeated once per Pod. |
| `DEPLOYMENT_PROGRESS_DEADLINE_EXCEEDED` | A rollout that ran past its deadline. |
| `DEPLOYMENT_REPLICA_FAILURE` | Pods that were rejected before they ever ran, with the API server's reason. |
| `DEPLOYMENT_ROLLOUT_IN_PROGRESS` | A rollout happening right now, so a replica gap is not mistaken for a fault. |
| `DEPLOYMENT_SCALED_TO_ZERO` / `DEPLOYMENT_PAUSED` | States the Deployment is in on purpose. |

### StatefulSet

| Identifier | What it detects |
| --- | --- |
| `STATEFULSET_ORDERED_ROLLOUT_BLOCKED` | The one replica that is holding up every replica after it — and which ones were never created because of it. |
| `STATEFULSET_CLAIM_NOT_BOUND` | A per-replica volume that will not bind, named by the replica it belongs to. |
| `STATEFULSET_CLAIM_NOT_FOUND` | A templated claim that does not exist. |
| `STATEFULSET_SERVICE_NOT_FOUND` | The governing Service is missing, so the Pods have no resolvable names — while looking perfectly healthy. |
| `STATEFULSET_SERVICE_NOT_HEADLESS` | It exists but has a cluster IP, which is the opposite of what a StatefulSet needs. |
| `STATEFULSET_UPDATE_ON_DELETE` / `_PARTITIONED` | A rollout that is waiting on purpose, not stalled. |
| `STATEFULSET_UNAVAILABLE_REPLICAS` | Fewer ready replicas than requested. |
| `STATEFULSET_SCALED_TO_ZERO` | Scaled to zero on purpose — and its claims are kept. |

### Ingress

| Identifier | What it detects |
| --- | --- |
| `INGRESS_SERVICE_NOT_FOUND` | A backend Service that does not exist. |
| `INGRESS_SERVICE_PORT_NOT_FOUND` | A port the backend Service does not expose. |
| `INGRESS_SERVICE_NO_ENDPOINTS` | A backend that publishes no ready endpoints — with the Pods behind it diagnosed. |
| `INGRESS_CLASS_NOT_FOUND` / `INGRESS_NO_CLASS` | No controller is going to serve this Ingress. |
| `INGRESS_NO_ADDRESS` | Nothing has published an address (reported at `possible` confidence — the API does not say why). |
| `INGRESS_NO_RULES` | An Ingress with nothing to route. |

### PersistentVolumeClaim

| Identifier | What it detects |
| --- | --- |
| `PVC_STORAGECLASS_NOT_FOUND` | The requested StorageClass does not exist. |
| `PVC_NO_DEFAULT_STORAGECLASS` | No class named, and the cluster has no default. |
| `PVC_PROVISIONING_FAILED` | The storage driver's own failure, relayed verbatim. |
| `PVC_NO_MATCHING_VOLUME` | Nothing could be bound to the claim. |
| `PVC_WAITING_FOR_CONSUMER` | A `WaitForFirstConsumer` claim waiting on purpose — reported as expected, not broken. |
| `PVC_NO_CONSUMER` | A `WaitForFirstConsumer` claim no Pod mounts, so nothing will ever trigger binding. |
| `PVC_LOST` | The volume the claim was bound to is gone. |
| `PVC_PENDING` | Not bound, and **no rule could explain it**. |

Each rule is documented under [`docs/rules/`](docs/rules/): what it detects, the evidence it uses, its confidence, and its limitations.

---

## How it works

```text
CLI
 │
 ▼
resource resolver          pod | svc | deploy | ing | pvc → Kind
 │
 ▼
collectors                 only the API calls the diagnosis can use
 ├─ the resource
 ├─ its events
 ├─ its ownership chain
 └─ related objects, when a symptom makes them relevant
 │
 ▼
snapshot                   normalized, read-only, no API access below this line
 │
 ▼
rules                      pure functions: snapshot → findings
 │
 ▼
diagnoses                  prioritised, root causes before consequences
 │
 ├─ text renderer
 └─ JSON renderer
```

Three concepts are kept strictly apart, and that separation is the whole point of the project:

- **Evidence** — a fact read from the Kubernetes API. Never inferred.
- **Diagnosis** — an interpretation of evidence, qualified by a confidence: `certain`, `likely` or `possible`.
- **Suggestion** — something a human may want to do. KubeWhy never does it.

More in [`docs/architecture.md`](docs/architecture.md).

---

## Security and privacy

KubeWhy is **read-only by design**.

- It performs only `get` and `list` requests. It never patches, deletes, scales, restarts, executes into containers, port-forwards or creates debug containers.
- It uses **your existing kubeconfig**: current context, namespace, authentication and exec credential plugins. No account, token or API key of its own.
- It does **not require cluster-admin**. When a related object cannot be read, the report says which part of the analysis is incomplete and continues with the rest.
- It **never reads Secret contents**. To check whether a referenced Secret exists, it asks the API server for object metadata only (`PartialObjectMetadata`), so the payload never reaches the process. No Secret client is exposed to collectors or rules.
- It does **not fetch application logs** (it suggests the `kubectl logs` command instead), and does not collect manifests.
- It makes **no external network requests**: no telemetry, no analytics, no cloud service, no LLM.

Details and the disclosure process: [`SECURITY.md`](SECURITY.md) and [`docs/security.md`](docs/security.md).

---

## JSON output

`-o json` is meant to be automated against. Rule identifiers and field names are treated as public API.

```console
$ kubectl why pod checkout-7c8cc8679-j9qd8 -n prod -o json
```

```json
{
  "resource": { "kind": "Pod", "namespace": "prod", "name": "checkout-7c8cc8679-j9qd8" },
  "status": "unhealthy",
  "headline": "Pod/checkout-7c8cc8679-j9qd8 is unhealthy",
  "diagnoses": [
    {
      "id": "POD_OOM_KILLED",
      "subject": { "kind": "Pod", "namespace": "prod", "name": "checkout-7c8cc8679-j9qd8" },
      "component": "checkout",
      "severity": "critical",
      "confidence": "certain",
      "summary": "Container \"checkout\" was terminated for exceeding its memory limit",
      "evidence": [
        { "source": "containerStatus", "field": "lastState.terminated.reason", "value": "OOMKilled" },
        { "source": "podSpec", "field": "resources.limits.memory", "value": "512Mi" }
      ],
      "possibleCauses": ["the application uses more memory than it was allocated"],
      "suggestions": [
        {
          "description": "Review the container's memory limit and the application's memory usage…",
          "commands": ["kubectl logs checkout-7c8cc8679-j9qd8 -n prod -c checkout --previous"]
        }
      ]
    }
  ]
}
```

Useful in CI:

```bash
kubectl why pod "$POD" -n "$NS" -o json | jq -r '.diagnoses[].id'
```

---

## Exit codes

| Code | Meaning |
| --- | --- |
| `0` | The resource was analysed and no issue was detected. |
| `1` | The resource was analysed and at least one issue was found. |
| `2` | KubeWhy could not run: invalid flags, no kubeconfig, or an API error. |
| `3` | The requested resource was not found. |
| `4` | The current user may not read the requested resource. |

A missing permission on a *related* object degrades the report; it does not produce exit code `4`.

---

## Versioning and stability

KubeWhy is pre-1.0, so the internal packages change freely. Four things do not, because people automate around them:

| Surface | Promise |
| --- | --- |
| CLI syntax | Flags and arguments are added, not removed or repurposed. |
| Rule identifiers | Added and deprecated, never silently renamed or reused for a different meaning. |
| JSON fields | Added, never renamed or removed. |
| Exit codes | Fixed. |

A change to any of them is released as a breaking change and explained in the release notes, whatever the version number happens to be.

**KubeWhy uses trunk-based development.** There is one branch, `main`, and everything that reaches it is released: the pull request title decides the version, the end-to-end scenarios are run against a real cluster, and the release is tagged and published minutes after the merge. There is no release branch, no release schedule and no version typed by hand. Details, and the reasoning, in [`docs/releasing.md`](docs/releasing.md).

## Roadmap

**v0.1 — feature complete, not yet released**

- [x] Pod diagnostics, text and JSON output, exit codes, RBAC degradation
- [x] Service diagnostics (selector, EndpointSlices, readiness, named ports)
- [x] Deployment diagnostics (rollouts, conditions, aggregated Pod failures)
- [x] StatefulSet diagnostics (ordered rollouts, per-replica volumes, headless Service)
- [x] Ingress diagnostics (Ingress → Service → port → EndpointSlice → Pod)
- [x] PersistentVolumeClaim diagnostics (storage class, binding mode, provisioning)
- [ ] Release binaries and Krew submission

**Later, deliberately not now**

`kubectl why namespace`, NetworkPolicy and DNS diagnostics, Gateway API, controller-specific Ingress knowledge, service meshes, CRDs, Helm and Argo CD awareness, metrics backends, and any optional AI explanation layer. KubeWhy earns trust by being deterministic and explainable first.

---

## Contributing

Contributions are very welcome, and adding a diagnosis rule is deliberately a small, well-bounded task: one rule, its tests, and one documentation page. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the walkthrough, and [`examples/broken/`](examples/broken/) for manifests that break on purpose so you can reproduce each case on a throwaway cluster.

```bash
make check     # gofmt, go vet, go test
make build
```

---

## License

[Apache License 2.0](LICENSE).
