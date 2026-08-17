# tori Constitution

<!--
  ②-form (D-12), authority revision AR-2026-08-17.1: this file does NOT own
  cross-repo invariants. It consumes the task Authority Snapshot and indexes
  only THIS repo's own enforced constraints. SoT for those is the rules
  themselves (Makefile gates / CI), not this prose.
-->

## Cross-repo authority — verified revision-pinned repository mirror

Cross-repo platform meaning is selected by the external Authority Router. For
`AR-2026-08-17.1` the scoped authority chain is:

- platform invariants: `Platform Spec Wiki — CURRENT / 1. constitution`
- platform structure / responsibility / call direction:
  `Platform Spec Wiki — CURRENT / 2. architecture`
- repository-portable invariant mirror: `HeaInSeo/NodeVault` —
  `docs/PLATFORM_MASTER_DESIGN.md §4.1–§4.10`
- mirror verification record: `HeaInSeo/NodeVault` —
  `docs/AUTHORITY_MIRROR_VERIFICATION.md`

tori does **not** treat NodeVault §4 as an independent platform canonical. A
task may consume the repository mirror for cross-repo invariant meaning only
when **all** of the following are true:

1. the task `Authority Snapshot` declares `AR-2026-08-17.1`;
2. the NodeVault verification record says `SYNC STATUS: VERIFIED`;
3. the mirror blob SHA matches the blob SHA recorded by that verification record;
4. every scoped/domain/component authority required by the tori task is
   explicitly present in the task `Authority Snapshot`;
5. no semantic conflict with the current Authority Router/upstream authority has
   been detected.

If any condition is missing, `STALE`, `UNKNOWN`, mismatched, or conflicting, stop
with `AUTHORITY_CONFLICT`; do not choose a source by timestamp, filename, or
search rank. **Revision equality alone is not sufficient.**

The current repository verification record covers platform invariants only. tori
work that depends on platform structure/ownership/call-direction or a specific
DB/rules/block/gRPC contract must carry the exact CURRENT architecture and
relevant scoped/component contract directly in the task `Authority Snapshot`.

## Process discipline (repo-operational — owned by this repo)

- **Deterministic gates are the guarantee.** Merge is decided by deterministic
  checks (tests, gosec, govulncheck, golangci-lint). LLM/agent review is
  **advisory**: a passing review never merges alone, a failing gate is never
  overridden.
- **Spec-anchored change**; **test-first** (behavioral changes ship with tests
  that fail before / pass after; `make test` runs the race-enabled variant);
  **Builder/Critic separation** (read-only Critic pass before merge).
- **Local verify (before a PR):**
  `make test lint lint-security-check vuln-check`.
- **Branch protection**: `master` lands via PR with required checks; no direct
  pushes.
- **Agent guidance**: see `AGENTS.md` for this repo's agent-operational
  guidance. This constitution does not duplicate or supersede it.

## Repo-local enforced constraints (derived index — NOT canonical)

> Derived index of THIS repo's own gates. Not canonical — SoT is the gate itself.

- **gosec** (IMPLEMENTED — `make lint-security-check`): static security analysis, blocking.
- **govulncheck** (IMPLEMENTED — `make vuln-check`): vulnerability scan, blocking.
- **golangci-lint** (IMPLEMENTED — `make lint`): lint gate.
- **race tests, core packages** (IMPLEMENTED — enforced by required check `core-baseline`, which runs `make test-core` with `-race -shuffle=on`, covering only `config`/`db`/`rules`/`block`/`cmd/...`): concurrency safety **for core packages only**. Packages outside that set (`service`, `transport/grpc`, `protoio`, `log`, root) have **no required race coverage** (the root guardrail runs without `-race`) — that is a PROPOSED gap, not an enforced guarantee.

## §1.10 — "do not record what you did not observe"

**Authority: CURRENT platform invariant under `AR-2026-08-17.1`. Enforcement in
this repo: PROPOSED where no deterministic local gate exists.** tori has no
deterministic rule that generally enforces this invariant today. The platform
invariant's authority status and this repo's local enforcement status are
separate axes.

## Governance

Cross-repo semantics cannot be amended by editing this constitution, a
repository mirror, or its verification record alone. They follow the task's
current Authority Snapshot; a new platform authority revision must be accepted
before repository mirrors are synchronized and independently re-verified.

**Version**: 2.1.0 | **Ratified**: 2026-08-02 | **Last Amended**: 2026-08-17
