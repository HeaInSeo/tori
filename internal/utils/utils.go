// Package utils holds small path/filesystem helpers vendored into tori so the
// repository has no external utility-module dependency.
package utils

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// IsEmptyString returns true if the string is empty or contains only whitespace, false otherwise
// 문자열이 공백만 있거나 비어 있으면 true, 아니면 false 반환
func IsEmptyString(s string) bool {
	return len(strings.TrimSpace(s)) == 0
}

// CheckPath validates and normalizes the provided file path.
// It returns an error if the file path is empty, otherwise it returns the cleaned file path.
// 파일 경로를 검사하고 정규화함. 비어 있으면 에러, 아니면 정리된 경로 반환
func CheckPath(filePath string) (string, error) {
	if IsEmptyString(filePath) {
		return "", fmt.Errorf("file path cannot be empty")
	}
	return filepath.Clean(filePath), nil
}

// FileExists reports whether the file at path exists. If it exists, the file's
// FileInfo is returned as well.
// 주어진 경로에 파일이 존재하는지 확인함. 있으면 true 와 FileInfo 반환, 없으면 false 반환
func FileExists(path string) (bool, os.FileInfo, error) {
	if IsEmptyString(path) {
		return false, nil, fmt.Errorf("path is empty")
	}

	fileInfo, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil, nil
		}
		return false, nil, err
	}
	return true, fileInfo, nil
}
