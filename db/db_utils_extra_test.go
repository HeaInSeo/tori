package db

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"testing/fstest"
)

func setupChangeFS() func() {
	old := sqlFiles
	sqlFiles = fstest.MapFS{
		"queries/insert_file.sql":  &fstest.MapFile{Data: []byte("INSERT INTO files VALUES (?,?,?)")},
		"queries/update_files.sql": &fstest.MapFile{Data: []byte("UPDATE files SET size=? WHERE id=?")},
		"queries/delete_files.sql": &fstest.MapFile{Data: []byte("DELETE FROM files WHERE id=?")},
	}
	return func() { sqlFiles = old }
}

func TestUpsertDelFile_Added(t *testing.T) {
	restore := setupChangeFS()
	defer restore()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	query := "INSERT INTO files VALUES (?,?,?)"
	mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(int64(1), "a", int64(10)).WillReturnResult(sqlmock.NewResult(1, 1))
	fc := FileChange{ChangeType: "added", FolderID: 1, Name: "a", DiskSize: 10}
	if err := fc.UpsertDelFile(context.Background(), db); err != nil {
		t.Fatalf("UpsertDelFile error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpsertDelFile_Modified(t *testing.T) {
	restore := setupChangeFS()
	defer restore()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	query := "UPDATE files SET size=? WHERE id=?"
	mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(int64(5), int64(2)).WillReturnResult(sqlmock.NewResult(1, 1))
	fc := FileChange{ChangeType: "modified", DiskSize: 5, FileID: 2}
	if err := fc.UpsertDelFile(context.Background(), db); err != nil {
		t.Fatalf("UpsertDelFile error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpsertDelFile_Removed(t *testing.T) {
	restore := setupChangeFS()
	defer restore()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	query := "DELETE FROM files WHERE id=?"
	mock.ExpectExec(regexp.QuoteMeta(query)).WithArgs(int64(3)).WillReturnResult(sqlmock.NewResult(1, 1))
	fc := FileChange{ChangeType: "removed", FileID: 3}
	if err := fc.UpsertDelFile(context.Background(), db); err != nil {
		t.Fatalf("UpsertDelFile error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestUpsertDelFile_Unknown(t *testing.T) {
	restore := setupChangeFS()
	defer restore()
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	fc := FileChange{ChangeType: "other"}
	if err := fc.UpsertDelFile(context.Background(), db); err == nil {
		t.Fatalf("expected error for unknown type")
	}
}

func TestUpsertDelFiles(t *testing.T) {
	restore := setupChangeFS()
	defer restore()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	q1 := "INSERT INTO files VALUES (?,?,?)"
	q2 := "UPDATE files SET size=? WHERE id=?"
	mock.ExpectExec(regexp.QuoteMeta(q1)).WithArgs(int64(1), "a", int64(10)).WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec(regexp.QuoteMeta(q2)).WithArgs(int64(5), int64(2)).WillReturnResult(sqlmock.NewResult(1, 1))
	changes := []FileChange{
		{ChangeType: "added", FolderID: 1, Name: "a", DiskSize: 10},
		{ChangeType: "modified", DiskSize: 5, FileID: 2},
	}
	if err := UpsertDelFiles(context.Background(), db, changes); err != nil {
		t.Fatalf("UpsertDelFiles error: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}
