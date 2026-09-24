package db

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	globallog "github.com/HeaInSeo/tori/log"
)

// SyncFolders reconciles the accepted inventory (DB) and the generated projection
// (FileBlock/DataBlock) with the source root, subject to the observation
// acceptance boundary (see acceptance.go):
//
//	observation → prove source/scope continuity → prove coverage completeness
//
//	scope UNKNOWN or coverage != COMPLETE
//	    → HOLD the whole authoritative application: no add/modify/remove DB
//	      mutation and no accepted projection overwrite. The previous accepted DB
//	      and projection are retained together and the result is a degraded HOLD.
//	scope CONFIRMED + COMPLETE
//	    → legacy add/modify/remove may proceed, but is not "accepted" until a
//	      crash-recoverable DB-mutation + projection-completion unit is durable.
//
// A crash/failure can never leave DB ahead of a stale projection silently
// accepted: a pending acceptance is detected and reconciled (rebuild-from-DB)
// before any "unchanged" is returned. datablock.pb is a generated projection of
// the accepted DB, not a canonical snapshot identity on its own.
//
// TDI-I2A: optional SyncOption values carry physical-access attributes that belong to the
// source access endpoint (see db/source.go). They are deliberately variadic so that adding
// an access attribute is never a semantic-scope change and never forces every existing
// caller to restate one.
func SyncFolders(ctx context.Context, db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string, opts ...SyncOption) (SyncResult, error) {
	var syncOpts syncOptions
	for _, opt := range opts {
		opt(&syncOpts)
	}

	// 1) Observe + classify BEFORE any mutation.
	res, err := observe(ctx, db, rootPath, foldersExclusions, filesExclusions)
	if err != nil {
		return SyncResult{}, err
	}
	if res.Outcome == OutcomeDegradedHold {
		globallog.Log.Warnf("SyncFolders HOLD (scope=%s coverage=%s): %s; 이전 스냅샷 유지",
			res.Scope, res.Coverage, res.Reason)
		return res, nil
	}

	// 1a) A confirmed observation with no recorded witness is either a true bootstrap
	//     or a validated legacy adoption (observe proved every accepted folder is
	//     present on disk). Backfill the continuity witness now so the source anchor
	//     becomes durable even when this run is otherwise unchanged; a wrong/empty
	//     mount never reaches here because observe HOLDs it first.
	//     establishWitnessIfBootstrap is a no-op once a witness exists.
	if err := establishWitnessIfBootstrap(ctx, db, rootPath); err != nil {
		globallog.Log.Errorf("source witness 백필 실패: %v", err)
		return SyncResult{}, err
	}

	// 1b) TDI-I2A: establish/refresh the durable source envelope (logical SourceID,
	//     immutable semantic revision, current physical access endpoint). This runs at
	//     the same proven-continuity point as the witness backfill, so a wrong/empty
	//     mount — already HELD by observe() above — can never be adopted as this
	//     source's endpoint.
	//
	//     TDI-I2B: the returned envelope is the exact source basis (SourceID + current
	//     SourceRevision + current endpoint) this observation runs under. Acceptance pins
	//     it with the target version and recovery consumes the pinned basis (see
	//     source_basis.go).
	//
	//     CredentialRef comes from the caller's configured access credential reference
	//     (WithAccessCredentialRef). The POSIX/shared-FS profile reaches the root through
	//     the mount and leaves it empty, but a profile that does configure one must have
	//     it recorded HERE, on the normal sync path — otherwise rotating the configured
	//     reference would silently leave CurrentEndpointID unchanged and contradict the
	//     endpoint contract this packet establishes.
	env, err := EnsureSourceEnvelope(ctx, db, SourceEnvelopeInput{
		RootDir:           rootPath,
		FoldersExclusions: foldersExclusions,
		FilesExclusions:   filesExclusions,
		CredentialRef:     syncOpts.accessCredentialRef,
	})
	if err != nil {
		globallog.Log.Errorf("source envelope 확립 실패: %v", err)
		return SyncResult{}, err
	}
	cur := currentSourceBasis(env)
	res.Source = cur

	// TDI-I4F: compute the current source scope and freeze its rule bases ONCE, before any
	// recovery/migration/mutation. The SAME frozen values drive drift comparison, legacy
	// migration, and acceptance, so a rule cannot change between a "check" read and a
	// separate "pin" read (TOCTOU-free). observe() already proved coverage COMPLETE. Per-
	// folder freeze failures are tolerated here (allBasesOK=false); they only HOLD on the
	// accept path (preflight), while drift/migration skip folders absent from the map.
	targetFolders, err := GetSubFolders(rootPath, foldersExclusions)
	if err != nil {
		globallog.Log.Errorf("GetSubFolders 실패: %v", err)
		return SyncResult{}, err
	}
	inScope := make(map[string]struct{}, len(targetFolders))
	for _, f := range targetFolders {
		inScope[f.Path] = struct{}{}
	}
	frozen, allBasesOK, freezeErr := freezeDiskBases(folderPaths(targetFolders), originNative)

	// 2) TDI-I4F v0.3 (F1): resolve the durable acceptance provenance. A pre-v0.3 DB with
	//    inventory but no recorded provenance is initialized to UNKNOWN_LEGACY (fail-closed);
	//    it is never inferred to be a fresh seed from accepted_version / datablock presence /
	//    row count. This provenance drives the legacy-migration branch below.
	provenance, err := resolveProvenanceForSync(ctx, db)
	if err != nil {
		return SyncResult{}, err
	}

	// 3) TDI-I4F v0.3 (F2) pending-basis guard: a pending recovery must have a provable
	//    frozen basis for EVERY in-scope accepted folder it will rebuild. Validate this
	//    per-folder — not by aggregate basis counts, which a crash mid-scoped-migration
	//    (target pinned only a subset) could bypass — BEFORE reconcile, so a missing basis
	//    surfaces a recoverable reclassification HOLD instead of a permanent
	//    ErrFrozenBasisUnavailable wedge inside reconcile. An empty in-scope inventory needs
	//    no basis and converges (empty-root pending initial acceptance).
	state, err := readAcceptanceState(ctx, db)
	if err != nil {
		return SyncResult{}, err
	}

	// 2b) TDI-I2B: every source basis this run may consume — the accepted version's and,
	//     while pending, the target's — must resolve inside the current envelope. A pin
	//     naming another SourceID, or a revision/endpoint the source never recorded, has
	//     unclear authority: HOLD before recovery or acceptance can build on it.
	//
	//     Recovery then rebuilds under the observation scope of the PINNED revision (target
	//     first, else accepted), never under the current config: a scope edit made while a
	//     prior acceptance was in flight must not reinterpret that acceptance.
	recoveryScope := inScope
	{
		acceptedVer, aErr := metaGetInt(ctx, db, metaKeyAcceptedVersion)
		if aErr != nil {
			return SyncResult{}, aErr
		}
		versions := []int64{acceptedVer}
		if state == acceptancePending {
			targetVer, tErr := metaGetInt(ctx, db, metaKeyTargetVersion)
			if tErr != nil {
				return SyncResult{}, tErr
			}
			versions = []int64{targetVer, acceptedVer}
		}
		pinnedScopeSet := false
		for _, v := range versions {
			b, ok, gErr := getSourceBasis(ctx, db, v)
			if gErr != nil {
				return SyncResult{}, gErr
			}
			if !ok {
				continue
			}
			if vErr := validateSourceBasis(ctx, db, env, b); vErr != nil {
				if !errors.Is(vErr, errSourceBasisUnresolved) {
					return SyncResult{}, vErr
				}
				res.Outcome = OutcomeDegradedHold
				res.Scope = ScopeUnknown
				res.Reason = fmt.Sprintf("source basis UNRESOLVED: %v; refusing to recover or accept on it", vErr)
				globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
				return res, nil
			}
			if state == acceptancePending && !pinnedScopeSet {
				scope, sErr := scopeForRevision(ctx, db, rootPath, b)
				if sErr != nil {
					return SyncResult{}, sErr
				}
				recoveryScope = scope
				pinnedScopeSet = true
			}
		}
	}

	if state == acceptancePending {
		targetVer, tErr := metaGetInt(ctx, db, metaKeyTargetVersion)
		if tErr != nil {
			return SyncResult{}, tErr
		}
		acceptedVer, aErr := metaGetInt(ctx, db, metaKeyAcceptedVersion)
		if aErr != nil {
			return SyncResult{}, aErr
		}
		if hold, missing, hErr := pendingBasisHold(ctx, db, recoveryScope, targetVer, acceptedVer); hErr != nil {
			return SyncResult{}, hErr
		} else if hold {
			res.Outcome = OutcomeReclassifyHold
			res.Reason = fmt.Sprintf("pending recovery without a provable frozen classification basis for in-scope folder %s; HOLD (reclassification/re-bootstrap required)", missing)
			globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
			return res, nil
		}
	}

	// 3) Reconcile any incomplete prior acceptance BEFORE we can report "unchanged".
	//    This closes I10: DB may have advanced while the projection stayed stale. The
	//    rebuild now projects from the FROZEN target basis, never disk rule.json, and
	//    (TDI-I2B) under the pinned source revision's scope, never the current config.
	reconciled, err := reconcileIfPending(ctx, db, rootPath, recoveryScope)
	if err != nil {
		globallog.Log.Errorf("reconcile 실패: %v", err)
		return SyncResult{}, err
	}
	if reconciled {
		globallog.Log.Warn("SyncFolders: 불완전 수용 감지 → 프로젝션 재생성(reconcile) 완료")
	}

	acceptedVer, err := metaGetInt(ctx, db, metaKeyAcceptedVersion)
	if err != nil {
		return SyncResult{}, err
	}

	// 4) TDI-I4F legacy migration bootstrap: a pre-I4F clean snapshot has accepted rows
	//    but no pinned classification basis. Adopt the current rules as migration-time
	//    semantics ONLY if they reproduce the accepted projection; otherwise HOLD.
	if held, reason, mErr := migrateLegacyBasisIfNeeded(ctx, db, rootPath, acceptedVer, provenance, inScope, frozen); mErr != nil {
		return SyncResult{}, mErr
	} else if held {
		res.Outcome = OutcomeReclassifyHold
		res.Reason = reason
		globallog.Log.Warnf("SyncFolders HOLD: %s", reason)
		return res, nil
	}

	// 4b) TDI-I2B: the accepted snapshot's pinned source basis. A projection restore of the
	//     ACCEPTED snapshot rebuilds under the scope of its pinned revision (acceptedScope),
	//     not the current config. sourceChanged means the current observation runs under a
	//     different SourceRevision than the accepted snapshot: adopting it is a NEW accepted
	//     version with its own pin, never a rewrite of the accepted one.
	acceptedPin, acceptedPinned, err := getSourceBasis(ctx, db, acceptedVer)
	if err != nil {
		return SyncResult{}, err
	}
	acceptedScope := inScope
	sourceChanged := false
	if acceptedPinned && !acceptedPin.sameSource(cur) {
		// Same revision means the same frozen scope, so the current enumeration already is
		// the accepted scope; only a revision change needs the pinned scope re-enumerated.
		sourceChanged = true
		if acceptedScope, err = scopeForRevision(ctx, db, rootPath, acceptedPin); err != nil {
			return SyncResult{}, err
		}
	}

	// 5) TDI-I4F drift: a rule-only semantic change on an in-scope accepted folder must
	//    not read as ordinary unchanged and must not adopt R2. Preserve accepted R1
	//    (restoring the projection from the accepted frozen basis if datablock.pb is
	//    missing) and HOLD.
	if driftFolder, fromRev, toRev, dErr := detectSemanticsDrift(ctx, db, acceptedVer, inScope, frozen); dErr != nil {
		return SyncResult{}, dErr
	} else if driftFolder != "" {
		// An incomplete prior reconcile this run leaves the snapshot pending (a vanished
		// folder was omitted from the projection). That incomplete state takes precedence over
		// a drift HOLD: reporting reclassify-hold here would mask an incomplete projection and
		// (with drift) never converge, since the drift branch returns before the diff/prune
		// path. Surface incomplete-pending instead; when the folder returns, the next sync's
		// reconcile completes and drift is re-evaluated properly. (A no-drift incomplete
		// reconcile never reaches this branch and still converges via the diff/prune path.)
		if st, sErr := readAcceptanceState(ctx, db); sErr != nil {
			return SyncResult{}, sErr
		} else if st == acceptancePending {
			res.Outcome = OutcomeIncompletePending
			res.Reason = fmt.Sprintf("pending reconciliation incomplete (an accepted folder is temporarily absent) while %s has classification-semantics drift; retry to converge", driftFolder)
			globallog.Log.Warnf("SyncFolders: %s", res.Reason)
			return res, nil
		}
		res.Outcome = OutcomeReclassifyHold
		if fromRev == "" {
			// Unverifiable basis: the folder has no recoverable accepted basis, so the
			// projection cannot be safely rebuilt — HOLD without touching it.
			res.Reason = fmt.Sprintf("accepted folder %s has no recoverable classification basis (unverifiable); reclassification required", driftFolder)
		} else {
			// Genuine R1->R2 drift: the accepted R1 basis exists, so if the projection is
			// missing, restore it crash-safely from that frozen basis (never on-disk R2) via
			// the shared primitive before holding. If an accepted folder vanished so the restore
			// is incomplete, surface incomplete/pending (retry to converge) rather than a clean
			// drift HOLD over an incomplete projection.
			outputDatablock := filepath.Join(rootPath, "datablock.pb")
			if _, statErr := os.Stat(outputDatablock); os.IsNotExist(statErr) {
				restored, rErr := publishAcceptedProjection(ctx, db, rootPath, acceptedScope)
				if rErr != nil {
					return SyncResult{}, rErr
				}
				if !restored {
					res.Outcome = OutcomeIncompletePending
					res.Reason = fmt.Sprintf("classification-semantics drift on %s, but the projection restore is incomplete (an accepted folder vanished); pending, retry to converge", driftFolder)
					globallog.Log.Warnf("SyncFolders: %s", res.Reason)
					return res, nil
				}
			}
			res.Reason = fmt.Sprintf("classification-semantics drift on %s (%s → %s): reclassification required; accepted R1 retained",
				driftFolder, shortRev(fromRev), shortRev(toRev))
		}
		globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
		return res, nil
	}

	// 6) Diff the confirmed+complete source against the accepted DB. The disk
	//    folderFiles byproduct is intentionally discarded: the projection is rebuilt
	//    from the accepted DB (see acceptWork), never from this raw disk snapshot.
	_, fDiff, fChange, err := DiffFolders(db, rootPath, foldersExclusions, filesExclusions)
	if err != nil {
		globallog.Log.Errorf("DiffFolders 실패: %v", err)
		return SyncResult{}, err
	}

	outputDatablock := filepath.Join(rootPath, "datablock.pb")
	_, statErr := os.Stat(outputDatablock)
	projectionMissing := os.IsNotExist(statErr)
	dataChanged := fDiff != nil || fChange != nil

	// A source revision change with no data change still falls through to acceptance: the
	// accepted snapshot names R1, so reporting "unchanged" under R2 would silently treat it
	// as R2. It is accepted as a new version pinned to R2; the R1 version keeps its pin.
	if !dataChanged && !sourceChanged {
		// A pre-I2B accepted snapshot has no source basis. The current revision reproduces
		// its inventory exactly (no data diff), so adopting it is proven, not inferred;
		// record the adoption as such. With a data diff it stays unpinned and the next
		// acceptance pins a new version natively.
		if !acceptedPinned {
			if n, cErr := countSemanticsAtVersion(ctx, db, acceptedVer); cErr != nil {
				return SyncResult{}, cErr
			} else if n > 0 {
				adopted := cur
				adopted.Origin = sourceBasisLegacyAdopted
				if pErr := pinSourceBasisTx(ctx, db, acceptedVer, adopted); pErr != nil {
					return SyncResult{}, pErr
				}
				globallog.Log.Warnf("TDI-I2B: adopted source revision %s for pre-I2B accepted snapshot v%d (reproduces accepted inventory)",
					shortRev(cur.RevisionID), acceptedVer)
			}
		}
		if reconciled {
			res.Outcome = OutcomeAcceptedUpdate
			res.Reconcile = true
			res.Reason = "reconciled incomplete prior acceptance"
			return res, nil
		}
		if !projectionMissing {
			globallog.Log.Info("all files and folders are same & datablock.pb exists; skipping update.")
			res.Outcome = OutcomeUnchanged
			return res, nil
		}
		// TDI-I4F firstRun laundering closure: a missing datablock.pb with an EXISTING
		// accepted frozen basis must be rebuilt from that basis (no version bump, no
		// clean-accept of on-disk R2). If no accepted basis is pinned yet, this is a
		// bootstrap (possibly with SaveFolders-seeded rows) — fall through to first
		// acceptance below, which freezes and pins the basis.
		hasAcceptedBasis, hErr := countSemanticsAtVersion(ctx, db, acceptedVer)
		if hErr != nil {
			return SyncResult{}, hErr
		}
		if hasAcceptedBasis > 0 {
			// Restore the missing projection crash-safely via the shared primitive: it records
			// pending BEFORE replacing datablock.pb and preserves the completeness signal, so a
			// crash mid-restore (or a folder vanishing before the rebuild) cannot leave an
			// incomplete projection that reads as clean/unchanged and hides an accepted folder.
			restored, rErr := publishAcceptedProjection(ctx, db, rootPath, inScope)
			if rErr != nil {
				return SyncResult{}, rErr
			}
			if !restored {
				res.Outcome = OutcomeIncompletePending
				res.Reason = "restored projection is incomplete (an accepted folder vanished); pending, retry to converge"
				globallog.Log.Warnf("SyncFolders: %s", res.Reason)
				return res, nil
			}
			res.Outcome = OutcomeAcceptedUpdate
			res.Reconcile = true
			res.Reason = "restored missing projection from accepted frozen basis"
			return res, nil
		}
	}

	// 7) Revalidate the folder set: DiffFolders is a fresh disk enumeration that ran after
	//    the basis was frozen. If a folder appeared in that window, the diff would add it
	//    to the DB while the frozen basis (and inScope) omit it — leaving it projected
	//    without a recoverable rule basis. HOLD and retry next sync (which re-freezes the
	//    current set) rather than accepting a folder whose basis was never frozen. Removals
	//    need no basis, so they are exempt.
	if missing := uncoveredDiffFolder(fDiff, fChange, frozen); missing != "" {
		res.Outcome = OutcomeDegradedHold
		res.Scope = ScopeConfirmed
		res.Coverage = CoverageComplete
		res.Reason = fmt.Sprintf("source folder set changed during sync (folder %s appeared after basis freeze); retry", missing)
		globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
		return res, nil
	}

	// 8) Accept as a crash-recoverable unit. The frozen target basis was preflighted once
	//    above; if any in-scope folder's rule was missing/invalid, HOLD BEFORE any DB
	//    mutation and retain the previous accepted DB + projection together (TDI-I4F §5).
	if !allBasesOK {
		res.Outcome = OutcomeDegradedHold
		res.Scope = ScopeConfirmed
		res.Coverage = CoverageComplete
		res.Reason = fmt.Sprintf("rule preflight HOLD: %v", freezeErr)
		globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
		return res, nil
	}
	targetBasis := basesSlice(frozen, folderPaths(targetFolders))

	// 8) Accept: pending + frozen target basis (one tx) → atomic DB mutation →
	//    projection rebuild from the pinned target basis → clean (promotes target basis).
	complete, err := acceptWork(ctx, db, rootPath, fDiff, fChange, targetBasis, cur, inScope)
	if err != nil {
		globallog.Log.Errorf("acceptWork 실패: %v", err)
		return SyncResult{}, err
	}
	if ctx.Err() != nil {
		globallog.Log.Warnf("SyncFolders 완료 이후 컨텍스트 취소 감지 (%v)", ctx.Err())
		return SyncResult{}, ctx.Err()
	}
	if !complete {
		// A folder vanished/left scope between the diff and the projection rebuild, so
		// acceptWork applied the DB mutation but left the snapshot PENDING (projection omits
		// an accepted row). Report OutcomeIncompletePending — NOT OutcomeAcceptedUpdate (the
		// snapshot is not clean) and NOT OutcomeDegradedHold (which asserts no mutation and a
		// retained prior snapshot, both false here). The next sync reconciles/prunes and
		// converges.
		res.Outcome = OutcomeIncompletePending
		res.Reason = "accepted DB advanced but projection is incomplete (a folder vanished/left scope mid-accept); pending, retry to converge"
		globallog.Log.Warnf("SyncFolders: %s", res.Reason)
		return res, nil
	}

	res.Outcome = OutcomeAcceptedUpdate
	res.Reconcile = reconciled
	if sourceChanged {
		res.Reason = fmt.Sprintf("source revision %s → %s accepted as a new snapshot version; the prior version keeps its pinned revision",
			shortRev(acceptedPin.RevisionID), shortRev(cur.RevisionID))
	}
	return res, nil
}

// folderPaths projects folder rows to their paths for rule-basis preflight.
func folderPaths(folders []Folder) []string {
	paths := make([]string, 0, len(folders))
	for _, f := range folders {
		paths = append(paths, f.Path)
	}
	return paths
}

// shortRev abbreviates a revision id for human-readable HOLD reasons.
func shortRev(rev string) string {
	const n = 12
	if len(rev) > n {
		return rev[:n]
	}
	return rev
}
