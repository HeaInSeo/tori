package db

import (
	"context"
	"database/sql"
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
func SyncFolders(ctx context.Context, db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) (SyncResult, error) {
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

	// 2) TDI-I4F pending-basis guard: a pending recovery must have a provable frozen
	//    classification basis. A pre-I4F pending state (or one whose basis was lost) has
	//    none, so reconciling would rebuild from the mutable on-disk rule — HOLD
	//    fail-closed instead of guessing historical semantics (TDI-I4F §7/§9).
	state, err := readAcceptanceState(ctx, db)
	if err != nil {
		return SyncResult{}, err
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
		nTarget, cErr := countSemanticsAtVersion(ctx, db, targetVer)
		if cErr != nil {
			return SyncResult{}, cErr
		}
		nAccepted, cErr := countSemanticsAtVersion(ctx, db, acceptedVer)
		if cErr != nil {
			return SyncResult{}, cErr
		}
		if nTarget == 0 && nAccepted == 0 {
			// Zero pinned bases is only a violation when there is accepted inventory to
			// protect. A pending INITIAL acceptance of an empty root legitimately pins no
			// basis (zero folders need none); reconcile must regenerate the empty
			// projection and converge rather than HOLD forever.
			folders, fErr := GetFoldersFromDB(db)
			if fErr != nil {
				return SyncResult{}, fErr
			}
			if len(folders) > 0 {
				res.Outcome = OutcomeReclassifyHold
				res.Reason = "pending recovery without a provable frozen classification basis; HOLD (re-bootstrap/reclassification required)"
				globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
				return res, nil
			}
		}
	}

	// 3) Reconcile any incomplete prior acceptance BEFORE we can report "unchanged".
	//    This closes I10: DB may have advanced while the projection stayed stale. The
	//    rebuild now projects from the FROZEN target basis, never disk rule.json.
	reconciled, err := reconcileIfPending(ctx, db, rootPath)
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
	if held, reason, mErr := migrateLegacyBasisIfNeeded(ctx, db, rootPath, acceptedVer); mErr != nil {
		return SyncResult{}, mErr
	} else if held {
		res.Outcome = OutcomeReclassifyHold
		res.Reason = reason
		globallog.Log.Warnf("SyncFolders HOLD: %s", reason)
		return res, nil
	}

	// Compute the in-scope source folders once: used to scope drift detection and, on
	// accept, to preflight the frozen basis. observe() already proved coverage COMPLETE.
	targetFolders, err := GetSubFolders(rootPath, foldersExclusions)
	if err != nil {
		globallog.Log.Errorf("GetSubFolders 실패: %v", err)
		return SyncResult{}, err
	}
	inScope := make(map[string]struct{}, len(targetFolders))
	for _, f := range targetFolders {
		inScope[f.Path] = struct{}{}
	}

	// 5) TDI-I4F drift: a rule-only semantic change on an in-scope accepted folder must
	//    not read as ordinary unchanged and must not adopt R2. Preserve accepted R1
	//    (restoring the projection from the accepted frozen basis if datablock.pb is
	//    missing) and HOLD.
	if driftFolder, fromRev, toRev, dErr := detectSemanticsDrift(ctx, db, acceptedVer, inScope); dErr != nil {
		return SyncResult{}, dErr
	} else if driftFolder != "" {
		outputDatablock := filepath.Join(rootPath, "datablock.pb")
		if _, statErr := os.Stat(outputDatablock); os.IsNotExist(statErr) {
			if _, rErr := regenerateProjectionFromDB(ctx, db, rootPath); rErr != nil {
				return SyncResult{}, rErr
			}
		}
		res.Outcome = OutcomeReclassifyHold
		res.Reason = fmt.Sprintf("classification-semantics drift on %s (%s → %s): reclassification required; accepted R1 retained",
			driftFolder, shortRev(fromRev), shortRev(toRev))
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

	if !dataChanged {
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
			if _, rErr := regenerateProjectionFromDB(ctx, db, rootPath); rErr != nil {
				return SyncResult{}, rErr
			}
			res.Outcome = OutcomeAcceptedUpdate
			res.Reconcile = true
			res.Reason = "restored missing projection from accepted frozen basis"
			return res, nil
		}
	}

	// 7) Accept as a crash-recoverable unit. Preflight the frozen target basis for every
	//    confirmed-source folder (computed once above) BEFORE any DB mutation: a
	//    missing/invalid rule HOLDs and the previous accepted DB + projection are retained
	//    together (TDI-I4F §5).
	targetBasis, err := freezeDiskBasis(folderPaths(targetFolders), originNative)
	if err != nil {
		res.Outcome = OutcomeDegradedHold
		res.Scope = ScopeConfirmed
		res.Coverage = CoverageComplete
		res.Reason = fmt.Sprintf("rule preflight HOLD: %v", err)
		globallog.Log.Warnf("SyncFolders HOLD: %s", res.Reason)
		return res, nil
	}

	// 8) Accept: pending + frozen target basis (one tx) → atomic DB mutation →
	//    projection rebuild from the pinned target basis → clean (promotes target basis).
	if err := acceptWork(ctx, db, rootPath, fDiff, fChange, targetBasis); err != nil {
		globallog.Log.Errorf("acceptWork 실패: %v", err)
		return SyncResult{}, err
	}
	if ctx.Err() != nil {
		globallog.Log.Warnf("SyncFolders 완료 이후 컨텍스트 취소 감지 (%v)", ctx.Err())
		return SyncResult{}, ctx.Err()
	}

	res.Outcome = OutcomeAcceptedUpdate
	res.Reconcile = reconciled
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
