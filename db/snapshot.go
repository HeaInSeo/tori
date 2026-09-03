package db

import (
	"context"
	"database/sql"
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

	// 2) Reconcile any incomplete prior acceptance BEFORE we can report "unchanged".
	//    This closes I10: DB may have advanced while the projection stayed stale.
	reconciled, err := reconcileIfPending(ctx, db, rootPath)
	if err != nil {
		globallog.Log.Errorf("reconcile 실패: %v", err)
		return SyncResult{}, err
	}
	if reconciled {
		globallog.Log.Warn("SyncFolders: 불완전 수용 감지 → 프로젝션 재생성(reconcile) 완료")
	}

	// 3) Diff the confirmed+complete source against the accepted DB. The disk
	//    folderFiles byproduct is intentionally discarded: the projection is rebuilt
	//    from the accepted DB (see acceptWork), never from this raw disk snapshot.
	_, fDiff, fChange, err := DiffFolders(db, rootPath, foldersExclusions, filesExclusions)
	if err != nil {
		globallog.Log.Errorf("DiffFolders 실패: %v", err)
		return SyncResult{}, err
	}

	outputDatablock := filepath.Join(rootPath, "datablock.pb")
	_, statErr := os.Stat(outputDatablock)
	firstRun := os.IsNotExist(statErr)

	needsUpdate := firstRun || fDiff != nil || fChange != nil
	if !needsUpdate {
		if reconciled {
			res.Outcome = OutcomeAcceptedUpdate
			res.Reconcile = true
			res.Reason = "reconciled incomplete prior acceptance"
			return res, nil
		}
		globallog.Log.Info("all files and folders are same & datablock.pb exists; skipping update.")
		res.Outcome = OutcomeUnchanged
		return res, nil
	}

	// 4) Accept as a crash-recoverable unit: pending → atomic DB mutation →
	//    projection rebuild from the accepted DB → clean.
	if err := acceptWork(ctx, db, rootPath, fDiff, fChange); err != nil {
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
