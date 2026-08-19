# Releasing

**KubeWhy uses trunk-based development.** There is one long-lived branch, `main`. There are no release branches, no maintenance branches and no long-lived feature branches. `main` is always releasable, and **everything that reaches `main` is released**.

Nobody types a version number, and nobody decides when to cut a release. The commits decide both.

One workflow runs on `main`. Everything a merge triggers is a job inside it, so
there is one place to look and one verdict.

```text
pull request ──squash──▶ main ──▶ Main workflow
                                       │
                    ┌──────────────────┴──────────────────┐
                    │                                     │
                 verify                                 plan
        tests on three platforms          what version do these commits
        gofmt, vet, golangci-lint              deserve, if any?
        shell tests                                      │
        release build, five platforms                    │
                    │                                    │
                    └──────────────┬─────────────────────┘
                                   ▼
                       end-to-end scenarios on kind
                            (only if releasing)
                                   │
                                   ▼
                        tag ──▶ publish ──▶ binaries
```

**Nothing is released unless everything upstream of it passed.** The release is
downstream of verification, not beside it: a failing test, a lint error, a
broken cross-platform build or a failing end-to-end scenario stops the pipeline
before anything is tagged.

Minutes after your pull request is merged, it is installable.

## Why trunk-based

The alternative — batching changes on a branch and releasing them together — buys a rehearsal window and costs the two things this project cares most about.

The first is **feedback**. Most of the bugs found in KubeWhy so far were found by pointing it at a real cluster and disliking the answer. A fix that sits unreleased for three weeks is a fix nobody is using, and a wrong diagnosis left in the wild is exactly the failure this project cannot afford.

The second is **honesty about what is tested**. A release branch is a configuration nobody develops against. Releasing `main` means the thing shipped is the thing everyone worked on.

The cost is that `main` has to actually be releasable, all the time. That is not a slogan here:

- Every pull request runs the tests on Linux, macOS and Windows, plus `gofmt`, `go vet`, `golangci-lint`, a release build for all five platforms, **and the end-to-end scenarios against a real cluster on kind**. That last one is not a luxury: the scenarios gate the release, so they have to gate the merge, or `main` can stop being releasable and take another commit to fix.
- **Every release is verified against a real Kubernetes cluster before it is tagged** as well. The tag is not created if the scenarios fail.
- Branch protection keeps unreviewed and unverified commits off `main`.

## What decides the version

The pull request title, because squash merging makes it the commit message on `main`. It must follow [Conventional Commits](https://www.conventionalcommits.org), and a workflow rejects it otherwise.

| Commit | Before 1.0 | After 1.0 |
| --- | --- | --- |
| `feat:` | minor — 0.1.0 → 0.2.0 | minor |
| `fix:`, `perf:`, `revert:` | patch — 0.1.0 → 0.1.1 | patch |
| `feat!:` or `BREAKING CHANGE:` in the body | minor, because 0.x promises nothing | major |
| `docs:`, `refactor:`, `test:`, `build:`, `ci:`, `chore:` | no release | no release |

When several land together, the strongest wins: one `feat` among ten `fix`es is a minor bump.

A new diagnostic rule is a `feat`. A rule that reported the wrong thing is a `fix`. The distinction reaches users: someone automating against a rule identifier reads the release notes to find out whether their pipeline's behaviour just changed.

The arithmetic lives in [`hack/next-version.sh`](../hack/next-version.sh), and because every release the project will ever cut goes through it, it has [its own tests](../hack/next-version.test.sh) that CI runs:

```bash
hack/next-version.sh --explain   # what would the next release be, and why
hack/next-version.test.sh        # is the arithmetic right
```

## The changelog

The GitHub release **is** the changelog. GoReleaser groups the commits since the previous tag by their Conventional Commits type, so there is no `CHANGELOG.md` in the repository to drift away from what actually shipped.

## Publishing the repository for the first time

```bash
gh repo create kubewhy --public --source=. --remote=origin --push

hack/setup-github.sh --repo <owner>/kubewhy --yes            # description, topics, labels, merge settings
hack/setup-github.sh --repo <owner>/kubewhy --yes --protect  # once CI has run once
```

Then upload `docs/assets/social-preview.png` under **Settings → General → Social preview**, the one thing the API does not expose.

The first push to `main` releases **v0.1.0**: with no previous tag, the history predates the commit convention, so deriving a version from it would be reading tea leaves. Every release after that is derived.

No secrets to configure. `GITHUB_TOKEN` covers tagging and publishing.

## Settings this depends on

| Setting | Why |
| --- | --- |
| Squash merging only | The pull request title becomes the commit on `main`, and that title is the version. With a merge commit the message is *"Merge pull request #12 from…"*, which parses as nothing and releases nothing. |
| Linear history | The version is derived from the commits between two tags. |
| Required status checks on `main` | "`main` is always releasable" has to be enforced, not hoped for. |

`hack/setup-github.sh` applies all three.

## Escape hatches

**Force a version.** Actions → Release → Run workflow, with a version such as `v0.2.0`. Useful for a first release, or to jump a number.

**Skip a release.** Land the change with a type that does not release (`docs`, `chore`, `refactor`). The change is on `main`; it simply waits for the next release to carry it.

**A bad release.** A published binary cannot be unpublished, so it is fixed by releasing over it. With trunk-based development that is one pull request titled `fix:` and a few minutes — which is the point.

**A failed release.** The tag has to exist before the build, because it is what the version is derived from, so a build that fails afterwards would leave a tag naming a release nobody can install — and the next run would read that tag as the last release and skip the version. The workflow deletes the tag when publishing fails, so the next attempt retries the same version instead of burning it.

## macOS notarization

The darwin binaries are ad-hoc signed by the Go linker, which is enough to
execute, but they are not notarized. Users who download through a browser get
the quarantine attribute and a Gatekeeper dialog offering to bin the file; the
README explains the three ways around it.

Removing that dialog entirely needs a paid Apple Developer account, a Developer
ID Application certificate and a notarization step in the release. GoReleaser
supports it, and it costs 99 USD a year. Until someone decides that is worth
it, the free ways around it are Krew and Homebrew: neither sets the quarantine
attribute, so a binary installed through either simply runs.

## Krew

The plugin is called `why` in the index, so it installs as `kubectl krew install why` and runs as `kubectl why`.

### The first submission, once

It happens **after** a release exists, because the manifest needs each archive's URL and its `sha256`.

```bash
hack/krew-manifest.sh --write        # generate krew/why.yaml from the latest release
kubectl krew install --manifest=krew/why.yaml   # install it exactly as the index CI will
kubectl why version
kubectl krew uninstall why
```

The manifest is generated rather than written. Five checksums copied by hand, once per release, is a transcription error waiting to happen — and one wrong character breaks the install for everyone except the person who made it.

Then fork [krew-index](https://github.com/kubernetes-sigs/krew-index), copy the file to `plugins/why.yaml`, and open a pull request. Its CI runs the same validation you just ran locally, and a maintainer reviews it.

Worth knowing before you write that pull request: `why-pending`, `why-fail` and `why-blocked` are already in the index. None of them is this — they explain one Pod state each; KubeWhy diagnoses six kinds and walks the relationships between them. Say so plainly in the description, because a reviewer looking at four plugins whose names start with "why" will ask.

### Every release after that, automatically

[krew-release-bot](https://github.com/rajatjindal/krew-release-bot) opens the update pull request itself. It reads [`.krew.yaml`](../.krew.yaml) at the repository root, resolves the checksums from the published archives, and raises the pull request against krew-index. The step is already in the release workflow, and it is allowed to fail: until the plugin is accepted there is nothing to update, and a release that has already shipped must not be marked failed for it.

The bot's template language offers exactly one variable, `{{ .TagName }}`, and no way to strip the leading `v`. That is why the release archives are named after the tag — `kubewhy_v0.2.0_linux_amd64.tar.gz` — rather than the bare version. Changing that naming breaks the bot silently, by generating URLs that 404.

Until the plugin is in the index, the README says Krew support is *planned*, and it should keep saying so.
