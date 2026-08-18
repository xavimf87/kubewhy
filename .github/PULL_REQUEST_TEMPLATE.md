## What this changes

<!-- One or two sentences. If it changes a diagnosis, show the output before and after. -->

## Why

<!-- The reasoning, not the diff. -->

> **The title of this pull request becomes the commit on `main`**, and decides the
> next version and the changelog entry. It must follow Conventional Commits —
> `feat:`, `fix:`, `perf:`, `docs:`, `refactor:`, `test:`, `build:`, `ci:`, `chore:` —
> with `!` before the colon for a breaking change. A workflow checks it.

## Checklist

- [ ] The title follows Conventional Commits, and its type matches the change
- [ ] `make check` passes (`gofmt`, `go vet`, `go test`)
- [ ] Tests added, including a **negative case**: the near-miss that must not produce this finding
- [ ] Documentation updated (`docs/rules/<ID>.md` and its index for a new rule)
- [ ] Golden files regenerated with `make golden` if the text output changed, and the diff reviewed

## Public API

Users automate against these, so they are treated as public API even before 1.0.

- [ ] No change to CLI syntax, rule identifiers, JSON fields or exit codes
- [ ] Something above changed — described here, with the reason:

<!-- Explain the change and why it cannot wait for a major version. -->

## Invariants

- [ ] Rules make no Kubernetes API calls
- [ ] Renderers make no interpretation
- [ ] Nothing claimed beyond what the evidence supports; uncertainty is in `PossibleCauses` and the confidence level
- [ ] No write operation, no Secret contents read, no log fetching, no outbound request other than to the API server
