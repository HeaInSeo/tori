# Source Envelope — design note v0.1 (TDI-I2A)

Status: implemented in `db/source.go`, established from `db/snapshot.go` (`SyncFolders`
step 1b). Consumption by acceptance is **I2B** — see "Consumption (TDI-I2B)" below.

## Problem

`RootDir` was three things at once:

1. the name of the source,
2. the observation scope,
3. the physical route used to reach it.

Because they were one value, two very different events were indistinguishable:

- an **operational** event (a mount moves, a credential rotates, a replica is promoted)
  looked like "this is a different source";
- a **semantic** event (the include/exclude scope is edited) looked like "same source,
  same meaning, nothing to record".

Neither can be reasoned about — or recovered from — while they share a representation.

## The three concepts

| Concept | Meaning | Changes when | Storage |
|---|---|---|---|
| `SourceID` | stable logical observation-domain identity | never (minted once) | `source_envelope` (singleton row) |
| `SourceRevision` | frozen observation **meaning** (logical scope / include-exclude semantics) | semantic scope is edited | `source_revisions`, append-only |
| `SourceAccessEndpoint` | physical access route + capability (`RootDir`, opaque credential ref) | relocation, credential rotation, failover | `source_endpoints`, append-only |

The two "current" pointers live in `snapshot_meta` (`source_current_revision`,
`source_current_endpoint`), matching the existing acceptance-state idiom.

### Invariants

- `SourceID` is **never** derived from a path, mount, host, PVC, credential, or the
  continuity witness token. It is random and namespaced `src-`.
- A revision is **content-addressed** (`sha256` of canonical JSON) and inserted with
  `ON CONFLICT DO NOTHING`. An existing revision therefore cannot be edited in place:
  the same meaning is the same row, different meaning is a different row. A snapshot
  accepted under a revision can never have that revision reinterpreted underneath it.
- Exclusion lists are matched as **sets** (`rules.ListFilesExclude` is "any pattern
  matches"), so the canonical form is sorted + de-duplicated and empty patterns are
  dropped. Reordering or repeating a pattern is **not** a semantic change.
- Endpoint attributes never enter a revision. Relocation and credential rotation append
  an endpoint and move the endpoint pointer; `SourceID` and the revision are untouched.

## Relationship to the continuity witness

`.tori_source_<token>` (see `observation_acceptance_boundary_v0.1.md`) proves that the
path in front of us is still the same *physical* source. It is endpoint **evidence**, not
identity: it is rotatable, re-minted on re-bootstrap, lives in the source filesystem, and
carries no observation meaning.

The dependency runs one way: proven witness continuity is what permits an endpoint change
to be adopted under the **existing** `SourceID`. Consequently the envelope is established
at the same point as the witness backfill in `SyncFolders` — *after* `observe()` has
already HELD a wrong or empty mount — so an unproven root can never be recorded as this
source's endpoint.

## Legacy adoption is explicit, not retroactive

A pre-I2A DB already holds accepted inventory. Minting a `SourceID` for it is an
**adoption**, and the envelope says so:

- `adoption_origin = legacy-adopted` (vs `bootstrap` for a genuinely fresh DB),
- `inventory_predates_id = 1`.

That second column is the honesty record: the identity is strictly younger than the data
it names. Nothing may claim the `SourceID` existed historically before adoption.

Establishment runs in a single transaction, so a crash leaves either the previous complete
envelope or the new complete envelope — never a `SourceID` without its revision/endpoint.
Re-running with unchanged input is a no-op.

## Explicitly out of scope for I2A

- `SyncFolders` **consuming** the envelope for acceptance decisions (I2B, below). In I2A
  step 1b was record-only.
- Content proof (I5P), Generation, Auto-Run, publication identity, UI.

## Consumption (TDI-I2B)

Implemented in `db/source_basis.go` and `SyncFolders` steps 2b/4b.

- **Pin per snapshot version.** `snapshot_source_basis(version)` records the exact
  `SourceID` + `SourceRevision` + endpoint a snapshot was accepted under. It is written
  under the target version in the same transaction as the pending transition and the I4F
  classification basis, and promoted by the same `accepted_version` flip. A pin is never
  rewritten (`ON CONFLICT DO NOTHING`).
- **Recovery uses the pinned revision.** A pending reconcile rebuilds under the frozen
  scope of the target's pinned revision (else the accepted one), read from
  `source_revisions` — never from the current config. A missing-projection restore uses
  the accepted snapshot's pinned scope the same way.
- **A newer revision is a new version.** When the current revision differs from the
  accepted snapshot's, the run is accepted as a new version with its own pin, even with no
  data diff; "unchanged" would silently re-label the R1 snapshot as R2.
- **Endpoint changes continue the revision.** A witness-proven relocation or a credential
  rotation keeps the same `SourceRevision`; rotation alone mints no version.
- **Unresolved pins HOLD.** A pin naming another `SourceID`, or a revision/endpoint the
  source never recorded, is a degraded HOLD before recovery or acceptance builds on it.
- **Legacy snapshots.** A pre-I2B accepted snapshot is pinned `legacy-adopted` only when
  the current revision reproduces its inventory with no data diff. Otherwise it stays
  unpinned (nothing is claimed about it) and the next acceptance pins natively.

`db/source_acceptance_test.go` (`TestI2B_*`) covers each rule through `SyncFolders`.

## Acceptance coverage

`db/source_test.go` pins the six required cases plus two boundary cases:

| Test | Case |
|---|---|
| `TestI2A_EndpointRelocationRetainsSourceID` | proven relocation retains `SourceID` |
| `TestI2A_UnprovenRootIsNotAdoptedAsEndpoint` | unproven root HOLDs and records nothing |
| `TestI2A_ScopeChangeCreatesNewRevisionWithoutMutatingOld` | semantic change appends a revision |
| `TestI2A_ExclusionReorderIsNotASemanticChange` | cosmetic config edit is not a revision |
| `TestI2A_CredentialRotationKeepsSourceIDAndRevision` | credential-only change moves only the endpoint |
| `TestI2A_LegacyBootstrapIsDurableIdempotentAndHonest` | legacy adoption is durable, idempotent, explicit |
| `TestI2A_EnvelopeSurvivesStoreReopen` | envelope survives a real close/reopen |
| `TestI2A_WitnessTokenCannotBeSourceID` | witness token cannot serve as identity |
| `TestI2A_EstablishCreatesNoPublicationIdentity` | no Generation/Auto-Run/publication identity |
