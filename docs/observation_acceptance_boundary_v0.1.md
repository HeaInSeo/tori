# Observation Acceptance Boundary (TDI-I1 + I10) — v0.1

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
  carry our token → scope UNKNOWN → HOLD. A first run with no recorded token is a
  legitimate bootstrap and establishes the witness as part of acceptance.

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
  accepted.

## Status surface

`SyncFolders` returns a `SyncResult` distinguishing `unchanged` /
`accepted-update` (including a reconcile) / `degraded-hold`, plus the observed
`Scope`/`Coverage` and a reason. The `sync` CLI reports all three.

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

The witness protects continuity only *after* the first accept records a token. On a
true first run (`recorded == ""`) there is no accepted state to protect, so the
bootstrap branch proceeds without a continuity check and establishes the witness on
whatever source is observed. Consequently, if the very first accept crashes after
the DB mutation but before the token is recorded, a restart against a *different*
mount would bootstrap there. This window is inherent to any witness bootstrap and
is out of scope for this boundary; migrating a pre-existing (pre-witness) DB has the
same one-time exposure until its first accept under this code. An interrupted
bootstrap that already wrote a marker file is handled: restart *adopts* the single
existing marker rather than writing a second one.

## Known deferred item

Projection generation (`rules`/`block`) emits its own side-files. `fileblock.csv`
and `*files.pb` are excluded by the default exclusions, but invalid-row folders
emit a timestamped `invalid_files_<ts>.txt` that the default exclusions do **not**
match (the exact-match `"invalid_files"` entry predates the timestamped name). Such
a folder would keep producing a tracked artifact, so the boundary would never
settle to `unchanged` for it. This is a pre-existing defect in the projection/rules
layer (outside this boundary's code surface) and is left for a follow-up that owns
`rules`/exclusion semantics.
