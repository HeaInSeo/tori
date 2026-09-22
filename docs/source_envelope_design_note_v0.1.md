# Source Envelope — design note v0.1 (TDI-I2A)

Status: implemented in `db/source.go`, established from `db/snapshot.go` (`SyncFolders`
step 1b). Consumption by acceptance is **I2B** and is deliberately not done here.

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

- `SyncFolders` **consuming** the envelope for acceptance decisions (I2B). Step 1b is
  record-only; no acceptance outcome changes in this packet.
- Content proof (I5P), Generation, Auto-Run, publication identity, UI.

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
