# Observation Acceptance Boundary (TDI-I1 + I10) — v0.2

Status: implemented (Lane A, bounded). Scope: the existing local/shared POSIX
snapshot path only. This note describes what the boundary guarantees and,
explicitly, what it does **not** claim.

## Why

`db.SyncFolders` used to run `DiffFolders → UpdateDB → GenerateFBs →
GenerateDataBlock` directly against whatever a single `os.ReadDir` returned. Two
gaps followed:

- **I1 — acceptance boundary.** A readable-but-wrong or empty source scope (e.g. a
  detached Lustre/NFS mount) could be accepted as the original source and wipe the
  accepted inventory + projection. A readable path is not proof of source
  continuity for shared/remote storage.
- **I10 — crash-recoverable acceptance.** The DB could advance while the projection
  (FileBlock/DataBlock) stayed stale, and `UpdateDB` could partially mutate rows.
  On the next run, an empty diff plus an existing `datablock.pb` looked like a
  complete accepted snapshot.

## Invariant

```
observation → prove intended source/scope continuity → prove coverage completeness

scope UNKNOWN  OR  coverage != COMPLETE
   → HOLD the whole authoritative application: no add/modify/remove DB mutation,
     no accepted FileBlock/DataBlock overwrite; retain the previous accepted DB +
     projection TOGETHER; surface a degraded HOLD.

scope CONFIRMED + COMPLETE
   → legacy add/modify/remove may proceed, but is not "accepted" until a
     crash-recoverable DB-mutation + projection-completion unit is durable.
```

## Mechanism (smallest that distinguishes the audited cases)

- **Source-continuity witness.** A zero-length marker file `.tori_source_<token>`
  at the source root; the token is also recorded in the DB (`snapshot_meta`).
  Continuity holds only when the recorded token is non-empty and exactly one marker
  with that token is present. Detection uses `os.ReadDir` only (never a
  content read), and the marker is a top-level file, so it is never part of the
  tracked inventory. An empty detached mount or a wrong replacement mount does not
  carry our token → scope UNKNOWN → HOLD. "No recorded token" is treated as a
  legitimate bootstrap **only when the accepted inventory is empty** (nothing to
  protect). A legacy/pre-feature DB holds accepted inventory with no recorded token;
  it is adopted **only** when the current source still carries every accepted folder
  (plausibly the same source), and the witness is then backfilled. If any accepted
  folder is missing, the source is refused (scope UNKNOWN → HOLD) rather than
  adopted, so a readable-but-wrong/empty mount can never wipe the accepted inventory.

- **Coverage encoding: {Complete, Partial}.** Every in-scope subfolder must be
  fully readable before any mutation; an unreadable subfolder → Partial → HOLD.

- **Acceptance / reconcile.** A durable dirty marker + accepted-version record in
  `snapshot_meta`, an atomic (single-transaction) DB mutation, and a
  rebuild-from-DB projection, applied in this order:

  1. mark `pending` + bump target version (durable "about to mutate");
  2. apply all DB row mutations atomically (single tx — all-or-nothing);
  3. rebuild FileBlock/DataBlock **from the accepted DB rows**;
  4. mark `clean` + set accepted = target.

  On restart, a `pending` marker forces a rebuild-from-DB **before** any
  "unchanged" can be returned. The rebuild is idempotent, so re-entry converges. A
  crash can therefore never leave `DB=S2, DataBlock=S1, next diff=empty` silently
  accepted. An **empty** accepted inventory is a representable state: it projects to
  an empty `DataBlock` (not an error), so removing the last tracked folder converges
  to `clean` instead of wedging the acceptance in a permanent `pending` state.

## Status surface

`SyncFolders` returns a `SyncResult` distinguishing `unchanged` /
`accepted-update` (including a reconcile) / `degraded-hold` / `reclassify-hold` /
`incomplete-pending`, plus the observed `Scope`/`Coverage` and a reason. The `sync` CLI
reports all five.

`incomplete-pending` means the DB mutation WAS applied but the projection could not be
completed in the same run (a folder vanished/left scope between the diff and the projection
rebuild), so the snapshot is left pending for the next sync to reconcile/prune. It is
deliberately distinct from `degraded-hold`, whose contract is "no mutation occurred, the
previous snapshot is retained intact" — here a mutation did occur and the projection is
intentionally incomplete until convergence.

`reclassify-hold` (added by TDI-I4F) is a classification-semantics HOLD, deliberately
distinct from both `unchanged` and `degraded-hold`. It fires when an accepted folder's
on-disk `rule.json` has drifted from the frozen classification-semantics basis it was
accepted under, or when that frozen basis cannot be recovered/verified. The accepted
snapshot (DB + projection) is retained; the drifted rule is neither adopted nor used,
and resolving it requires the later reclassification/publication lane (not this
boundary).

## Explicit non-claims (out of scope)

- `datablock.pb` is a **generated projection** of the accepted DB, not a canonical
  Tori "Generation" identity.
- This is **not** a platform-level source-identity authority: no
  SourceID/SourceRevision/Generation schema, no immutable Generation/publication
  tables, no remote-object (S3/GCS/Azure) adapters, no per-file SHA-256, no
  rename/copy/replica equivalence.
- The witness proves *repo-local continuity* for the local/shared POSIX profile
  only.

## Known limitation — bootstrap

Bootstrap (`recorded == ""`) is gated on an **empty accepted inventory**, so a
legacy/pre-feature DB with inventory is never treated as an unprotected bootstrap:
it is adopted only when every accepted folder is still present under the current
source, and refused (HOLD) otherwise. A legacy DB whose source *legitimately* lost
a whole folder before upgrade therefore HOLDs (retains prior state) rather than
accepting — safe, but it needs an explicit re-bootstrap to resume; that operator
path is a follow-up.

The residual window is only a *genuine* first run (empty inventory): if the very
first accept crashes after the DB mutation but before the token is recorded, a
restart against a *different* mount would bootstrap there. This window is inherent to
any witness bootstrap and is out of scope. An interrupted bootstrap that already
wrote a marker file is handled: restart *adopts* the single existing marker rather
than writing a second one.

## Determinism

The projection is rebuilt from the accepted DB in a deterministic order — folders
sorted by path, file names sorted ascending — so identical accepted inventory always
projects to the same FileBlock/row ordering regardless of DB row order or the history
by which the snapshot was reached (same data + same method = same result). The
`DataBlock.UpdatedAt` timestamp is the only intentionally non-deterministic field.

## Known limitation — reconcile vs. projection inputs

`reconcileIfPending` rebuilds the projection from the accepted DB. A folder that
*vanished* from disk during the pending window is skipped, and reconcile then reports
the rebuild as **incomplete** and does **not** mark the snapshot clean: it stays
`pending` so the normal diff/accept path in the same run prunes the folder and commits
a consistent, clean projection. Reconcile never declares clean while the projection
omits an accepted DB row, and never mutates the accepted DB itself.

Since **TDI-I4F**, projection is rebuilt from the **frozen per-folder classification
basis** pinned at acceptance (target basis while `pending`, accepted basis while
`clean`), never by re-reading the mutable on-disk `rule.json`. Consequently a later
removed/corrupted `rule.json` can no longer silently reinterpret — or hard-error — the
accepted projection: the accepted snapshot is rebuilt from its frozen basis. A folder
whose *frozen* basis is missing/unverifiable is surfaced fail-closed as a
`reclassify-hold` (not a hard error), and a `rule.json` that has drifted from the frozen
basis is a `reclassify-hold` too — in both cases the accepted DB + projection are
retained and reclassification is required. (Accepted rows that violate the rule contract
at *acceptance* time are still rejected by the rule preflight before the DB advances.)

Separately, if a crash left `pending` and the source is subsequently classified
UNKNOWN/PARTIAL, `SyncFolders` HOLDs at observation *before* reconciling, so the DB
may stay ahead of a stale projection until the source re-confirms. This is
self-healing (recoverable, not data loss); the HOLD reason should not be read as a
guarantee that DB and projection are mutually consistent during that window.

## Acceptance provenance (TDI-I4F v0.3)

The frozen-basis projection above removes silent reinterpretation **when an accepted basis
or accepted projection still exists**. It cannot, on its own, tell a genuinely fresh seed
(rows staged by the `SaveFolders` command, never accepted) apart from a legacy inventory
that *was* accepted before this feature but whose projection was later deleted — both look
identical on disk (`accepted_version==0` is possible for either, `datablock.pb` absent,
rows present, no pinned basis). Treating that state as a bootstrap would re-accept the
legacy inventory under the current, possibly changed, `rule.json`.

To close this, acceptance carries a **durable provenance** in `snapshot_meta`
(`acceptance_provenance`) with three values:

- `SEED_ONLY` — recorded when `SaveFolders` seeds an **empty** DB. A genuinely fresh seed
  that has never been accepted; its first clean acceptance may legitimately bootstrap.
- `ACCEPTED` — recorded **atomically with the clean/accepted transition** (in the same
  `commitClean` transaction), and when a legacy migration proves its adopted basis. The
  accepted snapshot + pinned basis are authoritative.
- `UNKNOWN_LEGACY` — the fail-closed default for any pre-provenance DB that already holds
  inventory. It is **never** inferred to be a fresh seed from `accepted_version`,
  `datablock.pb` presence, row count, or a missing basis. It may become authoritative only
  by reproducing an existing accepted projection exactly (migration-adopt); with no
  reproducible projection/basis it **HOLDs** (`reclassify-hold`).

Two related fail-closed refinements at the acceptance boundary:

- **Pending recovery** validates a resolvable frozen basis **per in-scope accepted folder**
  (target basis while pending, accepted basis as fallback), not by aggregate basis counts.
  A crash mid-scoped-migration that pins only a subset therefore surfaces a *recoverable*
  `reclassify-hold` rather than wedging reconcile on a permanent frozen-basis error.
- A folder that is **in scope but whose on-disk rule cannot be frozen** (missing/invalid)
  **and has no accepted basis** is surfaced as an unverifiable-basis `reclassify-hold`, not
  skipped as if out-of-scope and not returned as ordinary `unchanged` or a raw error.

This is provenance of acceptance-state history only — not a SourceID/Generation/publication
authority.

## Known deferred item

Projection generation (`rules`/`block`) emits its own side-files. `fileblock.csv`
and `*files.pb` are excluded by the default exclusions, but invalid-row folders
emit a timestamped `invalid_files_<ts>.txt` that the default exclusions do **not**
match (the exact-match `"invalid_files"` entry predates the timestamped name). Such
a folder would keep producing a tracked artifact, so the boundary would never
settle to `unchanged` for it. This is a pre-existing defect in the projection/rules
layer (outside this boundary's code surface) and is left for a follow-up that owns
`rules`/exclusion semantics.
