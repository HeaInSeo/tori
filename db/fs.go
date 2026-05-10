package db

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// GetSubFolders 특정 디렉토리 내의 서브 폴더(디렉토리)들을 읽어 Folder 구조체 슬라이스로 반환함.
// IMPORTANT: exclusions 목록에 포함된 이름과 정확히 일치하거나 접두어로 시작하는 폴더는 제외함.
func GetSubFolders(rootPath string, exclusions []string) ([]Folder, error) {
	var folders []Folder

	// 지정된 디렉토리 내의 항목들을 읽음 (Go 1.16 이상: os.ReadDir 사용)
	entries, err := os.ReadDir(rootPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read directory %s: %w", rootPath, err)
	}

	// 디렉토리 이름을 비교하여 제외할지 결정하는 헬퍼 함수
	excludeDir := func(dirName string, exclusions []string) bool {
		for _, ex := range exclusions {
			// 디렉토리 이름이 정확히 일치하는 경우 제외
			if dirName == ex {
				return true
			}
		}
		return false
	}

	// 각 항목에 대해 처리
	for _, entry := range entries {
		// 폴더(디렉토리)가 아닌 경우 건너뜀
		if !entry.IsDir() {
			continue
		}

		folderName := entry.Name()
		// exclusions 목록에 있는 폴더이면 건너뜀 (디렉토리 이름도 비교)
		if excludeDir(folderName, exclusions) {
			continue
		}

		// 폴더 전체 경로 생성
		folderPath := filepath.Join(rootPath, folderName)

		info, err := entry.Info()
		if err != nil {
			return nil, fmt.Errorf("failed to get folder info for %s: %w", folderPath, err)
		}

		// Folder 구조체 생성 (TotalSize 와 FileCount 는 기본값 0)
		folder := Folder{
			ID:          0, // DB 삽입 전이므로 0
			Path:        folderPath,
			TotalSize:   0,
			FileCount:   0,
			CreatedTime: info.ModTime().Format("2006-01-02 15:04:05"),
		}

		folders = append(folders, folder)
	}

	return folders, nil
}

// GetCurrentFolderFileInfo 특정 디렉토리 내의 파일들을 읽어 전체 파일 개수, 총 크기와 각 파일의 메타데이터를 수집.
// Go 1.16부터 도입된 os.ReadDir, DirEntry.Info()를 사용하여 시스템 콜을 최소화함. dirPath 여기서 이 폴더는 조사하고자 하는 자신의 폴더 path 임.
func GetCurrentFolderFileInfo(dirPath string, exclusions []string) (Folder, []File, error) {
	var folder Folder
	var files []File

	// 디렉토리 내 파일 목록 읽기 (Go 1.16 이상에서는 os.ReadDir 사용)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return folder, nil, fmt.Errorf("failed to read directory %s: %w", dirPath, err)
	}

	totalSize := int64(0)
	fileCount := int64(0)

	// excludeFiles 는 dirName 이 exclusions 목록에 있는 항목과 정확히 일치하거나,
	// 만약 exclusions 항목이 "*.확장자" 형태이면, dirName 에 해당 확장자가 포함되어 있으면 true 를 반환함.
	excludeFiles := func(fileName string, exclusions []string) bool {
		for _, ex := range exclusions {
			// 패턴이 "*.<ext>" 형식이면, 해당 확장자가 dirName 내에 존재하는지 확인함.
			if strings.HasPrefix(ex, "*.") {
				ext := ex[1:] // 예: "*.pb" -> ext 는 ".pb"
				if strings.Contains(fileName, ext) {
					return true
				}
			} else {
				// 일반적인 정확한 비교
				if fileName == ex {
					return true
				}
			}
		}
		return false
	}

	// 각 엔트리(파일)에 대해 처리
	for _, entry := range entries {
		if entry.IsDir() {
			continue // 하위 디렉토리는 무시
		}
		fileName := entry.Name()

		// 제외 목록에 있는 파일이면 건너뛰기
		if excludeFiles(fileName, exclusions) {
			continue
		}

		// 파일 전체 경로 생성
		filePath := filepath.Join(dirPath, fileName)

		// 파일 정보 가져오기 (os.ReadDir 가 반환하는 DirEntry 의 Info() 사용)
		info, err := entry.Info()
		if err != nil {
			return folder, nil, fmt.Errorf("failed to get file info for %s: %w", filePath, err)
		}

		size := info.Size()
		totalSize += size
		fileCount++

		// File 구조체 생성 (ID 및 FolderID는 DB 삽입 후 업데이트 가능)
		// IMPORTANT sqlite 에서 AUTOINCREMENT 로 시작하도록 하였음. 따라서 0 이 들어간것은 DB 에 들어가기 전 데이터임.
		fileRecord := File{
			ID:          0, // 아직 DB에 저장되지 않았으므로 0 또는 추후 채움
			FolderID:    0, // folder 삽입 후 업데이트
			Name:        fileName,
			Size:        size,
			CreatedTime: info.ModTime().Format("2006-01-02 15:04:05"),
			Path:        dirPath, // Path 필드에 실제 파일 경로를 채움
		}
		files = append(files, fileRecord)
	}

	// IMPORTANT Folder 구조체 생성 (ID는 DB 삽입 후 업데이트)
	folder = Folder{
		ID:          0,
		Path:        dirPath,
		TotalSize:   totalSize,
		FileCount:   fileCount,
		CreatedTime: time.Now().Format("2006-01-02 15:04:05"),
	}

	return folder, files, nil
}

// CompareFolders 디스크와 DB의 폴더 정보를 비교하여 변경 사항이 있는지 확인함.
func CompareFolders(db *sql.DB, rootPath string, foldersExclusions, filesExclusions []string) (bool, []Folder, []FolderDiff, error) {
	// 디스크에서 서브 폴더 목록 조회 (통계는 아래 루프에서 filesExclusions 기준으로 한 번만 계산)
	diskFolders, err := GetSubFolders(rootPath, foldersExclusions)
	if err != nil {
		return false, nil, nil, fmt.Errorf("failed to get subfolders from disk: %w", err)
	}

	// DB 에서 폴더 정보 조회
	dbFolders, err := GetFoldersFromDB(db)
	if err != nil {
		return false, nil, nil, fmt.Errorf("failed to get folders from DB: %w", err)
	}

	// DB 폴더 정보를 경로 기준으로 맵으로 구성 (폴더 경로를 키로 사용)
	dbFolderMap := make(map[string]Folder)
	for _, folder := range dbFolders {
		dbFolderMap[folder.Path] = folder
	}

	var diffs []FolderDiff
	// 디스크의 각 폴더에 대해 파일 통계를 갱신한 후 DB와 비교
	for _, diskFolder := range diskFolders {
		updatedFolder, _, err := GetCurrentFolderFileInfo(diskFolder.Path, filesExclusions)
		if err != nil {
			return false, nil, nil, fmt.Errorf("failed to get folder details for %s: %w", diskFolder.Path, err)
		}

		if dbFolder, ok := dbFolderMap[diskFolder.Path]; !ok {
			// DB에 해당 폴더 정보가 없는 경우 FolderID를 0으로 처리
			diffs = append(diffs, FolderDiff{
				ChangeType:    "added",
				FolderID:      0,
				Path:          diskFolder.Path,
				CreatedTime:   diskFolder.CreatedTime,
				DiskTotalSize: updatedFolder.TotalSize,
				DBTotalSize:   0,
				DiskFileCount: updatedFolder.FileCount,
				DBFileCount:   0,
			})
		} else {
			// DB에 해당 폴더 정보가 있는 경우
			if updatedFolder.TotalSize != dbFolder.TotalSize || updatedFolder.FileCount != dbFolder.FileCount {
				diffs = append(diffs, FolderDiff{
					ChangeType:    "modified",
					FolderID:      dbFolder.ID,
					Path:          diskFolder.Path,
					DiskTotalSize: updatedFolder.TotalSize,
					DBTotalSize:   dbFolder.TotalSize,
					DiskFileCount: updatedFolder.FileCount,
					DBFileCount:   dbFolder.FileCount,
				})
			}
		}
	}

	// DB에만 있고 디스크에서 사라진 폴더를 감지
	diskFolderSet := make(map[string]struct{}, len(diskFolders))
	for _, f := range diskFolders {
		diskFolderSet[f.Path] = struct{}{}
	}
	for _, dbFolder := range dbFolders {
		if _, ok := diskFolderSet[dbFolder.Path]; !ok {
			diffs = append(diffs, FolderDiff{
				ChangeType: "removed",
				FolderID:   dbFolder.ID,
				Path:       dbFolder.Path,
			})
		}
	}

	unchanged := len(diffs) == 0
	return unchanged, diskFolders, diffs, nil
}

// normalizeTimeStr는 go-sqlite3 드라이버가 DATETIME 컬럼을 RFC3339("T"/"Z" 포함)로
// 반환하는 경우에 대비해 "YYYY-MM-DD HH:MM:SS" 형식으로 정규화함.
func normalizeTimeStr(s string) string {
	if len(s) > 19 {
		s = s[:19]
	}
	if len(s) == 19 && s[10] == 'T' {
		s = s[:10] + " " + s[11:]
	}
	return s
}

// CompareFiles  파일 비교.
func CompareFiles(db *sql.DB, folderPath string, filesExclusions []string) (bool, []File, []FileChange, error) {
	// 디스크의 파일 정보 조회
	_, diskFiles, err := GetCurrentFolderFileInfo(folderPath, filesExclusions)
	if err != nil {
		return false, nil, nil, fmt.Errorf("failed to get folder details for %s: %w", folderPath, err)
	}
	// DB의 파일 정보 조회 (해당 Folder 에 해당하는)
	dbFiles, err := GetFilesByPathFromDB(db, folderPath)
	if err != nil {
		return false, nil, nil, fmt.Errorf("failed to get DB files for folder %s: %w", folderPath, err)
	}

	// 파일 이름을 키로 하는 맵 생성 (디스크와 DB 각각)
	diskMap := make(map[string]File)
	for _, f := range diskFiles {
		diskMap[f.Name] = f
	}
	dbMap := make(map[string]File)
	for _, f := range dbFiles {
		dbMap[f.Name] = f
	}

	var changes []FileChange
	// 디스크에만 있는 파일 (추가된 파일)
	for name, diskF := range diskMap {
		if dbF, ok := dbMap[name]; !ok {
			changes = append(changes, FileChange{
				ChangeType:  "added",
				FileID:      0,
				FolderID:    diskF.FolderID,
				Name:        name,
				CreatedTime: diskF.CreatedTime,
				DiskSize:    diskF.Size,
				DBSize:      0,
				Path:        diskF.Path,
			})
		} else {
			// size 또는 mtime 이 다른 경우 수정으로 판단 (mtime은 포맷 정규화 후 비교)
			if diskF.Size != dbF.Size || normalizeTimeStr(diskF.CreatedTime) != normalizeTimeStr(dbF.CreatedTime) {
				changes = append(changes, FileChange{
					ChangeType:  "modified",
					FileID:      dbF.ID,
					FolderID:    dbF.FolderID,
					Name:        name,
					CreatedTime: diskF.CreatedTime,
					DiskSize:    diskF.Size,
					DBSize:      dbF.Size,
					Path:        diskF.Path,
				})
			}
		}
	}
	// DB 에만 있는 파일 (삭제된 파일), Path 가 없음. 실제로 없으므로.
	for name, dbF := range dbMap {
		if _, ok := diskMap[name]; !ok {
			changes = append(changes, FileChange{
				ChangeType: "removed",
				FileID:     dbF.ID,
				FolderID:   dbF.FolderID,
				Name:       name,
				DiskSize:   0,
				DBSize:     dbF.Size,
				Path:       "",
			})
		}
	}
	unchanged := len(changes) == 0
	return unchanged, diskFiles, changes, nil
}

// GetFoldersInfo 지정한 Folder 배열에 대해, 각 Folder 의 TotalSize 와 FileCount 값을 계산하여 업데이트함.
// exclusions: 해당 폴더 내에서 제외할 파일 목록.
func GetFoldersInfo(rootPath string, exclusions []string) ([]Folder, error) {

	folders, err := GetSubFolders(rootPath, exclusions)
	if err != nil {
		return nil, fmt.Errorf("failed to get subfolders: %w", err)
	}

	for i, folder := range folders {
		// 각 폴더에 대해 GetCurrentFolderFileInfo 를 호출하여 파일 통계 계산 (파일 정보는 무시)
		updatedFolder, _, err := GetCurrentFolderFileInfo(folder.Path, exclusions)
		if err != nil {
			return nil, fmt.Errorf("failed to compute stats for folder %s: %w", folder.Path, err)
		}
		// 계산된 TotalSize 와 FileCount 로 업데이트
		folders[i].TotalSize = updatedFolder.TotalSize
		folders[i].FileCount = updatedFolder.FileCount
		// CreatedTime 등 다른 값도 필요하면 업데이트 가능 (옵션)
		folders[i].CreatedTime = updatedFolder.CreatedTime
	}
	return folders, nil
}

// DeleteFiles 전달받은 파일 경로 목록에서 2개 이상의 파일이 존재하면 모두 삭제
func DeleteFiles(files []string) error {
	if len(files) > 1 {
		for _, filePath := range files {
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("failed to delete file (%s): %w", filePath, err)
			}
		}
	}
	return nil
}

// DeleteFilesByPattern 주어진 폴더 내에서 지정한 glob 패턴에 매칭되는 파일들을 검색해서 삭제함
// 만약 매칭된 파일이 2개 이상이면, 해당 파일들을 모두 삭제
func DeleteFilesByPattern(folder, pattern string) error {
	files, err := SearchFilesByPattern(folder, pattern)
	if err != nil {
		return fmt.Errorf("패턴 검색 실패 (%s): %w", pattern, err)
	}

	// 매칭된 파일이 여러 개인 경우에만 삭제 수행
	if len(files) > 1 {
		for _, filePath := range files {
			if err := os.Remove(filePath); err != nil {
				return fmt.Errorf("파일 삭제 실패 (%s): %w", filePath, err)
			}
		}
	}
	return nil
}

// SearchFilesByPattern 주어진 폴더 내에서 지정한 glob 패턴에 매칭되는 파일들을 검색
// 검색 결과로 매칭된 파일 경로들의 배열을 반환합니다.
func SearchFilesByPattern(folder, pattern string) ([]string, error) {
	fullPattern := filepath.Join(folder, pattern)
	matches, err := filepath.Glob(fullPattern)
	if err != nil {
		return nil, fmt.Errorf("패턴 검색 실패 (%s): %w", pattern, err)
	}
	return matches, nil
}

// FileExistsExact 주어진 폴더 내에서 정확한 파일명이 존재하는지 확인. 별도로 FileExists 가 있지만 그냥 이걸 씀.
func FileExistsExact(folder, fileName string) (bool, error) {
	path := filepath.Join(folder, fileName)
	if _, err := os.Stat(path); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, fmt.Errorf("파일 체크 실패 (%s): %w", path, err)
	}
}

// ExtractFileNames 변환.
func ExtractFileNames(files []File) []string {
	names := make([]string, 0, len(files))
	for _, f := range files {
		names = append(names, f.Name)
	}
	return names
}
