# Production-readiness work record

Objective: make Configra production-usable and suitable for open-source
distribution under Apache-2.0. This record supplements, not replaces or narrows,
the [production acceptance contract](production-readiness.md).

## Requirements and evidence still needed

| Requirement | Required evidence | Current state |
| --- | --- | --- |
| Apache-2.0 distribution | License/notice in source, nested module, binaries and images; third-party license inventory | Source license and policy files added; distribution/notices audit remains |
| Open-source maintenance | Contribution/security policies, working private disclosure, continuous checks and dependency updates | Policies added; private reporting verified enabled on service/SDK; continuous checks/dependency updates still need completion |
| MySQL 8.0.22 support | Exact-version integration, migration and restore tests; unchanged capacity gate | Required by the user; not replaced with an 8.4-only baseline |
| MySQL 8.4 comparison | Official compatibility/upgrade sources, separately identified runtime evidence | [Primary-source comparison](research/mysql-8.0.22-and-8.4-compatibility.md) completed; runtime evidence absent |
| Complete authenticated mutation audit | Authenticated rejections have durable, value-free receipts; logical Operation identities survive replay; final image/failure drills pass | Gateway capture and credential-identifier repair implemented; [live HTTP, E2E and UI checks passed](audit-hardening-2026-09-12.md); final image and recovery stress/fault checks remain |
| Bounded management inventories | Server-side limits/pagination, consumer behavior and scale/query evidence | Frontend pagination exists; backend bounding remains |
| Key recovery and lifecycle | Correct/wrong-key drills, usable rotation procedure, CA/client lifecycle checks | Existing lifecycle tests; rotation/recovery acceptance still needs review |
| Production process and Kubernetes | Exact release images, readiness/shutdown, TLS pass-through, rollout, CSI/sync and dependency failure tests | Prior evidence exists; final candidate must be revalidated |
| 1000 QPS for ten minutes | Production image, real encrypted Vault reference, mTLS, 2 CPU/512 MiB limit, unchanged gate, leakage checks and recorded host | Last current-candidate warmup failed; not accepted |
| Usable instructions, not slogans | Clean-environment walkthroughs of quickstart, SDK, installation, Kubernetes and backup/restore; failures fixed | [Local protocol and SDK walkthrough](documentation-verification.md) executed; startup failure fixed in development source, remaining guides still need full execution |
| Chinese and English website | Same-page language switching, corresponding guides, locale-correct links/metadata, build and content checks | 28 pages built; type check and 32 static tests passed; published as website commit `0d42d33`; browser acceptance still open |
| Release integrity | Clean commit/tag mapping, tests, license/notices/SBOM and downloadable multi-platform artifacts | Preview artifacts exist; stable release not approved |

## Working rules

- Keep MySQL 8.0.22 in scope. A newer compatibility target adds evidence; it does
  not erase the requested legacy-version support.
- Use disposable fixtures, never the user's production database or cluster.
- A successful build, narrow smoke test or old benchmark is not evidence that
  all acceptance requirements pass for the current candidate.
- Do not lower resource, throughput, security or leakage thresholds to obtain a
  green result. Record failures and investigate them.
- Documentation commands must identify their working directory, prerequisites,
  placeholders and expected outcome. Mark anything not yet executed as such.
- Publish new tags for changed releases; do not rewrite already-published tags.
- The goal stays open until each requirement has direct, current evidence.

## First-party licensing versus distribution

[Direct-dependency research](research/direct-dependency-licenses.md) identifies
MPL-2.0 for the MySQL driver, file-specific MIT/Apache licensing for YAML and
required upstream notices. The old SDK module archives lack LICENSE/NOTICE files.
Before a new release, complete the transitive inventory, retain required texts and
source-access information in artifacts, publish a license-bearing SDK version,
and verify what recipients actually download. Do not describe a top-level Apache
file as completion of the distribution audit.
