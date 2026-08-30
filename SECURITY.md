# Security Policy

## Supported versions

fraise is pre-1.0. Only the most recent release receives security fixes; there are no backported patch lines yet. From 1.0.0 onward this section will name a supported window explicitly.

| Version | Supported |
|---|---|
| 0.1.x (latest release) | yes |
| anything older | no |

## Reporting a vulnerability

**Please do not open a public issue for a security problem.**

Report it privately through GitHub:
<https://github.com/FraiseHQ/fraise/security/advisories/new>

If you would rather use email, write to **contact@getfraise.dev** — put `fraise` in the subject line.

A useful report includes the fraise version (`GET /` returns it), the configuration you were running, an FQL query or HTTP request that triggers the problem, and what you expected to happen instead. A reproducer is worth more than a description.

fraise is maintained by one person, so please read these timelines as honest intent rather than a contractual SLA:

- **Acknowledgement within 72 hours.** If you have not heard back in 5 days, assume the message went astray and ping again.
- **An initial assessment within 10 days** — whether it is in scope, and a rough severity.
- **A fix or a documented mitigation within 90 days** for anything confirmed.

## Coordinated disclosure

Please give me 90 days from acknowledgement before publishing. If a fix lands sooner the advisory goes out sooner, and I am happy to coordinate a joint announcement. If a vulnerability is already being exploited in the wild, tell me and we compress the timeline rather than waiting.

Fixes ship as a GitHub Security Advisory with a CVE where one is warranted, a patch release, and a CHANGELOG entry. Reporters are credited by name unless they ask not to be.

## Current security model — read this before reporting

fraise is a database, and several properties that look like vulnerabilities are documented limitations of the current release. Reports about the following are welcome as issues, but they are not treated as vulnerability disclosures:

- **There is no authentication or authorisation.** Any client that can reach the HTTP port can read and write every graph. API keys, per-key graph ACLs and TLS are scheduled for 0.3.0.
- **The `@N` graph selector is not a trust boundary.** It separates data for convenience, not security. Until per-key ACLs exist, treat every graph in a process as equally readable by anyone who can reach it.
- **Bind to localhost.** The supported deployment for the current release is a local process or a sidecar on a private network. Exposing the port to an untrusted network is not a supported configuration.
- **There is no durability yet.** Data lives in memory and is lost on restart. Persistence arrives in 0.3.0.

What *is* in scope, at any release: remote crashes or panics reachable from a well-formed request, memory exhaustion triggerable by a single bounded request, any path where one graph's data is returned for a query addressed to another, query parsing that reads or writes outside the intended structures, dependency vulnerabilities that are actually reachable from fraise's code, and anything in the release pipeline that would let a third party alter a published artifact.

## Supply chain

Releases are built by GitHub Actions from a tag on `main`, published with SLSA provenance attestation, and the container images are built by native multi-arch runners. If you find a way to influence a published binary or image without write access to the repository, that is a vulnerability and I want to hear about it.
