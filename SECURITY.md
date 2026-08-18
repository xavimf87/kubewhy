# Security policy

## What KubeWhy does with your cluster

KubeWhy is a read-only CLI that runs on your machine with your credentials.

- It uses your current kubeconfig: context, namespace, authentication and exec credential plugins.
- It performs **read-only** Kubernetes API operations (`get`, `list`). It never creates, updates, patches, deletes, scales, restarts or evicts anything.
- It never executes commands inside containers, attaches, port-forwards, or creates ephemeral or debug containers.
- It **does not require cluster-admin**. Missing permissions on related objects degrade the report rather than failing it.
- It **never reads Secret contents**. Secret existence is checked with a metadata-only request (`PartialObjectMetadata`), so the payload is never sent to the client. No Secret client is exposed to collectors or rules.
- It does not fetch container logs. It prints the `kubectl logs` command instead.
- It transmits nothing anywhere: no telemetry, no analytics, no update checks, no cloud service, no LLM API. The only host it talks to is your Kubernetes API server.

The technical detail behind each of these claims is in [`docs/security.md`](docs/security.md).

## Supported versions

KubeWhy is pre-1.0. Security fixes are applied to the latest released version.

| Version | Supported |
| --- | --- |
| Latest release | ✅ |
| Older releases | ❌ |

## Reporting a vulnerability

Please **do not open a public issue** for a security vulnerability.

> **Maintainers:** replace this block with your preferred private channel before the first public release — GitHub's [private security advisories](https://docs.github.com/en/code-security/security-advisories/guidance-on-reporting-and-writing-information-about-vulnerabilities/privately-reporting-a-security-vulnerability) for this repository, or a dedicated email address. No address is invented here.

When you report, please include:

- the KubeWhy version (`kubectl why version`) and how it was installed;
- the Kubernetes version and distribution;
- what happens, and what you expected instead;
- the smallest reproduction you can share.

You can expect an acknowledgement of your report and, once a fix is available, credit in the release notes unless you prefer otherwise.

## What counts as a vulnerability

Examples of reports that are clearly in scope:

- KubeWhy performing any write operation against the API server.
- Secret contents, tokens, certificates or credentials appearing in any output, including `--verbose` and `-o json`.
- Any outbound network request to a host other than the configured Kubernetes API server.
- Reading or writing files outside what kubeconfig loading requires.
- A crash or hang triggered by attacker-controlled object content (for example, a crafted event message).

A wrong or incomplete diagnosis is a bug, not a vulnerability — please open a normal issue for it.
