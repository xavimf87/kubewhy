# Releasing

Releases are cut from `main`, and nobody types a version number.

## Publishing the repository for the first time

```bash
gh repo create kubewhy --public --source=. --remote=origin --push

hack/setup-github.sh --repo <owner>/kubewhy --yes    # description, topics, labels, merge settings
hack/setup-github.sh --repo <owner>/kubewhy --yes --protect   # once CI has run once
```

Then, in the web interface, the two things the API does not expose well:

* **Settings → General → Social preview** — upload `docs/assets/social-preview.png`.
* **Settings → Actions → General → Workflow permissions** — allow GitHub Actions to create and approve pull requests.

Finally, cut the first release as described under [The first release](#the-first-release).

## How it works

```text
pull request  ──squash──▶  main  ──▶  release-please keeps a Release PR open
                                              │
                                    merge that PR
                                              │
                                    tag + GitHub release + changelog
                                              │
                                    GoReleaser attaches the binaries
```

1. Pull requests are **squash-merged**, so the pull request title becomes the commit on `main`. A workflow checks that title against [Conventional Commits](https://www.conventionalcommits.org), because it is what everything below reads.
2. On every push to `main`, [release-please](https://github.com/googleapis/release-please) works out what the next version should be and keeps a single pull request open: *"chore(main): release 0.2.0"*. It holds the version bump and the changelog entry.
3. **Merging that pull request is the act of releasing.** It tags the commit, publishes the GitHub release with the changelog, and updates `CHANGELOG.md`.
4. The same workflow run then builds the binaries for all five platforms and attaches them, with checksums, to that release.

Steps 3 and 4 share one workflow run deliberately: a tag pushed by `GITHUB_TOKEN` does not start another workflow, so a separate tag-triggered job would never fire and the release would ship empty.

## What decides the version

| Commit type | Effect before 1.0 | Effect after 1.0 |
| --- | --- | --- |
| `feat:` | minor — 0.1.0 → 0.2.0 | minor |
| `fix:`, `perf:` | patch — 0.1.0 → 0.1.1 | patch |
| `feat!:` or `BREAKING CHANGE:` | minor, since 0.x makes no stability promise | major |
| `docs:`, `refactor:`, `test:`, `build:`, `ci:`, `chore:` | no release | no release |

A new diagnostic rule is a `feat`. A rule that reported the wrong thing is a `fix` — and it is worth being precise about which, because a user reading the changelog wants to know whether a finding they automate against changed.

Anything that changes **CLI syntax, a rule identifier, a JSON field or an exit code** is breaking, whatever the version number says. Mark it `!` and say so in the body.

## The first release

The commit history predates this convention, so release-please has nothing to derive a first version from. Cut `0.1.0` once, by hand, with the mechanism release-please provides:

```bash
git commit --allow-empty -m "chore: release 0.1.0" -m "Release-As: 0.1.0"
git push origin main
```

That opens the Release PR for `v0.1.0`. Merge it, and the process above takes over for good. Do not leave `release-as` in `release-please-config.json`; it would pin every future release to the same number.

## What has to be true for it to work

* **Settings → Actions → General → Workflow permissions**: *Allow GitHub Actions to create and approve pull requests*. Without it, release-please cannot open its pull request and the workflow fails with a permissions error that does not say so.
* **Squash merging only.** With merge commits, the message release-please reads is *"Merge pull request #12 from ..."*, which parses as nothing and releases nothing. `hack/setup-github.sh` turns the other merge methods off.
* No secrets to configure. `GITHUB_TOKEN` is enough for everything, including publishing binaries.

## Checking a release before it exists

```bash
goreleaser check                      # the configuration is valid
goreleaser build --snapshot --clean   # all five platforms build
```

CI runs both on every pull request, so a release cannot be broken by a change that was reviewed.

## Krew

Krew is a separate, manual submission, and it comes **after** a release exists, because the manifest needs the archive URLs and their checksums:

1. Copy the real version and the `sha256` values from the release's `checksums.txt` into `dist/krew/why.yaml`.
2. Validate locally: `kubectl krew install --manifest=dist/krew/why.yaml`.
3. Open a pull request against [krew-index](https://github.com/kubernetes-sigs/krew-index).

Until that pull request is merged, the README says Krew support is *planned*, and it should keep saying so.

## If something goes wrong

The release is a normal GitHub release. Deleting it and its tag resets the state; release-please recomputes from the commit history on the next push. Yanking a released binary is not possible, so a bad release is fixed by cutting the next one — which, with `fix:`, takes one pull request.
