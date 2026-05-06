package db

import (
	"context"
	"database/sql"
	"fmt"
)

// structures

type File struct {
	ID          int64  `db:"id"`
	FolderID    int64  `db:"folder_id"`
	Name        string `db:"name"`
	Size        int64  `db:"size"`
	CreatedTime string `db:"created_time"` // sting 으로 해도 충분
	Path        string `db:"-"`            // DB 매핑에서 완전히 제외
}

type Folder struct {
	ID          int64  `db:"id"`
	Path        string `db:"path"`
	TotalSize   int64  `db:"total_size"`
	FileCount   int64  `db:"file_count"`
	CreatedTime string `db:"created_time"` // string 으로 해도 충분
}

// FolderDiff 는 디스크와 DB의 Folder 통계가 다른 경우의 차이를 나타냄.
// ChangeType: "added" | "modified" | "removed". 빈 문자열은 "modified"와 동일하게 처리됨 (하위 호환).
type FolderDiff struct {
	ChangeType    string // "added", "modified", "removed"
	FolderID      int64  // DB에 있는 폴더의 ID (없으면 0)
	Path          string // Folder 경로
	CreatedTime   string // 디스크상의 폴더 mtime ("added" 일 때만 사용)
	DiskTotalSize int64  // 디스크상의 총 크기
	DBTotalSize   int64  // DB에 저장된 총 크기
	DiskFileCount int64  // 디스크상의 파일 개수
	DBFileCount   int64  // DB에 저장된 파일 개수
}

// FileChange 는 특정 Folder 내에서 디스크와 DB의 파일 정보가 다를 경우 그 차이를 나타냄.
// TODO 여기에다가 Path 하나 넣어 두자. 왜냐하면 어디에 있는 파일이 변경되었는지 파악해야 함으로.
// 이 Path 를 통해서 Folder 테이블을 검색할 수 있음.
// 지금 키가 되는 FileId, FolderId 자체가 들어가지 않으니, 이건 FileChange 만들때 그냥 빈공가느로 남겨두자, 향후 쓰일 수도 있으니. Ptah 를 넣자.
// DB 자체를 건드는게 아님. 중요.
type FileChange struct {
	ChangeType string // "added", "removed", "modified"
	// DB에 이미 존재하는 파일의 경우 FileID와 FolderID를 기록합니다.
	FileID      int64
	FolderID    int64
	Name        string // 파일 이름
	CreatedTime string // 디스크상의 파일 mtime ("added" 일 때만 사용)
	DiskSize    int64  // 디스크상의 파일 크기
	DBSize      int64  // DB에 저장된 파일 크기 (추가된 경우 0)
	Path        string // 파일이 속한 폴더의 경로
}

// UpsertFolder FolderDiff 정보를 기반으로 DB의 폴더 정보를 업데이트하거나, 없으면 삽입 또는 삭제.
func (fd *FolderDiff) UpsertFolder(ctx context.Context, db *sql.DB) error {
	if fd.ChangeType == "removed" {
		// 디스크에서 사라진 폴더: ON DELETE CASCADE 로 files 도 함께 삭제됨
		if err := execSQL(ctx, db, "delete_folder.sql", fd.FolderID); err != nil {
			return fmt.Errorf("failed to delete folder id %d, path %s: %w", fd.FolderID, fd.Path, err)
		}
		return nil
	}
	if fd.FolderID == 0 {
		// DB에 해당 폴더 정보가 없는 경우: 새 레코드 삽입
		if err := execSQL(ctx, db, "insert_folder.sql", fd.Path, fd.DiskTotalSize, fd.DiskFileCount, fd.CreatedTime); err != nil {
			return fmt.Errorf("failed to insert folder for path %s: %w", fd.Path, err)
		}
	} else {
		// DB에 해당 폴더 정보가 있는 경우: 업데이트
		if err := execSQL(ctx, db, "update_folder.sql", fd.DiskTotalSize, fd.DiskFileCount, fd.FolderID); err != nil {
			return fmt.Errorf("failed to update folder id %d, path %s: %w", fd.FolderID, fd.Path, err)
		}
	}
	return nil
}

// UpsertDelFile FileChange 정보를 기반으로 DB의 파일 정보를 업데이트하거나, 없으면 삽입 또는 삭제.
func (fc *FileChange) UpsertDelFile(ctx context.Context, db *sql.DB) error {
	switch fc.ChangeType {
	case "added":
		if err := execSQL(ctx, db, "insert_file.sql", fc.FolderID, fc.Name, fc.DiskSize, fc.CreatedTime); err != nil {
			return fmt.Errorf("failed to insert file %s: %w", fc.Name, err)
		}
	case "modified":
		if err := execSQL(ctx, db, "update_file.sql", fc.DiskSize, fc.CreatedTime, fc.FileID); err != nil {
			return fmt.Errorf("failed to update file %s: %w", fc.Name, err)
		}
	case "removed":
		if err := execSQL(ctx, db, "delete_file.sql", fc.FileID); err != nil {
			return fmt.Errorf("failed to delete file %s: %w", fc.Name, err)
		}
	default:
		return fmt.Errorf("unknown change type: %s", fc.ChangeType)
	}
	return nil
}
