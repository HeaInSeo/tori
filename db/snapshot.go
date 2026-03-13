package db

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"

	"github.com/seoyhaein/tori/block"
	globallog "github.com/seoyhaein/tori/log"
)

// SyncFolders 는 DB 스냅샷 비교부터 DataBlock 파일 생성까지 모두 처리 TODO SyncFolders, DiffFolders 들ㅇ가는 입력 파라미터 수정할 필요 있음.
func SyncFolders(ctx context.Context, db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) (bool, error) {
	// 1) DiffFolders 호출
	folderFiles, fDiff, fChange, err := DiffFolders(db, rootPath, foldersExclusions, filesExclusions)
	if err != nil {
		globallog.Log.Errorf("DiffFolders 실패: %v", err)
		return false, err
	}

	// 2) datablock.pb 경로 준비
	outputDatablock := filepath.Join(rootPath, "datablock.pb")
	_, statErr := os.Stat(outputDatablock)
	firstRun := os.IsNotExist(statErr)

	// 3) 업데이트 필요 여부 판단
	needsUpdate := firstRun || fDiff != nil || fChange != nil
	if !needsUpdate {
		globallog.Log.Info("all files and folders are same & datablock.pb exists; skipping update.")
		return false, nil
	}

	// 4) DB 업데이트
	if fDiff != nil || fChange != nil {
		if err := UpdateDB(ctx, db, fDiff, fChange); err != nil {
			globallog.Log.Errorf("UpdateDB 실패: %v", err)
			return false, err
		}
		if ctx.Err() != nil {
			globallog.Log.Warnf("SyncFolders 종료: 컨텍스트 취소 감지 (%v)", ctx.Err())
			return false, ctx.Err()
		}
	}

	// 5) FileBlock 생성 (api 패키지로 위임)
	fbs, err := block.GenerateFBs(folderFiles)
	if err != nil {
		globallog.Log.Errorf("GenerateFBs 실패: %v", err)
		return false, err
	}
	if ctx.Err() != nil {
		globallog.Log.Warnf("SyncFolders 종료: 컨텍스트 취소 감지 (%v)", ctx.Err())
		return false, ctx.Err()
	}

	// 6) DataBlock 저장
	if err := block.GenerateDataBlock(fbs, outputDatablock); err != nil {
		globallog.Log.Errorf("GenerateDataBlock 실패 (%s): %v", outputDatablock, err)
		return false, err
	}
	if ctx.Err() != nil {
		globallog.Log.Warnf("SyncFolders 완료 이후 컨텍스트 취소 감지 (%v)", ctx.Err())
		return false, ctx.Err()
	}

	return true, nil
}
