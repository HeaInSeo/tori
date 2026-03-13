package db

import (
	"context"
	"database/sql"
	"errors"
	"io/fs"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/DATA-DOG/go-sqlmock"
)

// 참고: "Unable to resolve table 'test'" 또는 다른 경고는 SQL 구문에서 참조하는 테이블이 실제 데이터베이스에 없어서 발생함.
// 이 테스트에서는 fstest.MapFS 와 sqlmock 을 사용하여 SQL 실행을 시뮬레이션하므로 실제 테이블 존재 여부는 무시해도 됨.
// testFS는 테스트용 가상 파일 시스템
var testFS = fstest.MapFS{
	"queries/test_valid.sql":        &fstest.MapFile{Data: []byte("INSERT INTO test (id) VALUES (1);")},
	"queries/test_empty.sql":        &fstest.MapFile{Data: []byte("")},
	"queries/test_fail.sql":         &fstest.MapFile{Data: []byte("UPDATE test SET id = 1;")},
	"queries/test_select_valid.sql": &fstest.MapFile{Data: []byte("SELECT 1;")},
	"queries/test_select_fail.sql":  &fstest.MapFile{Data: []byte("SELECT * FROM non_existing_table;")},
}

// 각 테스트 시작 전에 sqlFiles 를 테스트용 파일 시스템으로 재정의
func initTestFS(t *testing.T) {
	t.Helper()
	old := sqlFiles
	sqlFiles = testFS
	t.Cleanup(func() {
		sqlFiles = old
	})
}

// -------------------
// Transaction 관련 테스트 (execSQLTx, execSQLTxNoCtx)
// -------------------

func TestExecSQLTx_FileNotFound(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	mock.ExpectRollback()

	// 존재하지 않는 파일을 지정하여 에러가 발생하는지 검증
	err = execSQLTx(context.Background(), tx, "nonexistent.sql")
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read SQL file") {
		t.Fatalf("unexpected error message: %v", err)
	}

	// 여기서 명시적으로 rollback을 호출해서 기대를 만족시킴.
	if rErr := tx.Rollback(); rErr != nil && !errors.Is(rErr, sql.ErrTxDone) {
		logger.Warnf("failed to tx Rollback: %v", rErr)
	}

	mock.ExpectClose()
	if cErr := db.Close(); cErr != nil {
		logger.Warnf("failed to db close: %v", cErr)
	}

	// 이제 모든 기대치가 충족되었는지 확인
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}
}

func TestExecSQLTx_EmptyFile(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	// rollback 호출에 대한 기대 등록
	mock.ExpectRollback()
	// 실제로 rollback 호출을 위한 defer 구문 추가
	defer func() {
		if rErr := tx.Rollback(); rErr != nil && !errors.Is(rErr, sql.ErrTxDone) {
			logger.Warnf("failed to tx Rollback: %v", rErr)
		}
	}()

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	err = execSQLTx(context.Background(), tx, "test_empty.sql")
	if err == nil {
		t.Fatalf("expected error for empty SQL file, got nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestExecSQLTx_Success(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	// 트랜잭션 시작 기대 등록 및 실행
	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	// 테스트용 SQL 파일을 읽어 쿼리 문자열 생성
	content, err := fs.ReadFile(sqlFiles, "queries/test_valid.sql")
	if err != nil {
		t.Fatalf("failed to read test_valid.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	// Exec 호출에 대한 기대 등록
	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(1, 1))
	// 함수 호출
	err = execSQLTx(context.Background(), tx, "test_valid.sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 모든 기대치가 충족되었는지 확인
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	// rollback 호출에 대한 기대 등록
	mock.ExpectRollback()
	// 실제로 rollback 호출을 위한 defer 구문 추가
	defer func() {
		if rErr := tx.Rollback(); rErr != nil && !errors.Is(rErr, sql.ErrTxDone) {
			logger.Warnf("failed to tx Rollback: %v", rErr)
		}
	}()

	// db.Close() 호출 기대 등록 및 defer 처리 (마지막에 호출됨)
	mock.ExpectClose()
	defer func() {
		if cErr := db.Close(); cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestExecSQLTx_QueryExecutionError(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_fail.sql")
	if err != nil {
		t.Fatalf("failed to read test_fail.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	expectedErr := errors.New("execution error")

	mock.ExpectExec(query).WillReturnError(expectedErr)
	err = execSQLTx(context.Background(), tx, "test_fail.sql")
	if err == nil {
		t.Fatalf("expected error from ExecContext, got nil")
	}
	if !strings.Contains(err.Error(), "SQL execution failed") {
		t.Fatalf("unexpected error message: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	// rollback 호출에 대한 기대 등록
	mock.ExpectRollback()
	// 실제로 rollback 호출을 위한 defer 구문 추가
	defer func() {
		if rErr := tx.Rollback(); rErr != nil && !errors.Is(rErr, sql.ErrTxDone) {
			logger.Warnf("failed to tx Rollback: %v", rErr)
		}
	}()

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestExecSQLTxNoCtx_Success(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectBegin()
	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("failed to begin transaction: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_valid.sql")
	if err != nil {
		t.Fatalf("failed to read test_valid.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	//mock.ExpectExec(query).WillReturnResult(sqlmock.NewResult(1, 1))
	// regexp.QuoteMeta()는 query 에 포함된 모든 특수문자를 이스케이프(escape)해서, 해당 문자열을 정규표현식에서도 리터럴(literal)로 인식하게 만든다.
	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(1, 1))
	err = execSQLTxNoCtx(tx, "test_valid.sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	// rollback 호출에 대한 기대 등록
	mock.ExpectRollback()
	// 실제로 rollback 호출을 위한 defer 구문 추가
	defer func() {
		if rErr := tx.Rollback(); rErr != nil && !errors.Is(rErr, sql.ErrTxDone) {
			logger.Warnf("failed to tx Rollback: %v", rErr)
		}
	}()

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

// -------------------
// DB 관련 테스트 (execSQL, execSQLNoCtx)
// -------------------

func TestExecSQL_FileNotFound(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	err = execSQL(context.Background(), db, "nonexistent.sql")
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read SQL file") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestExecSQL_EmptyFile(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	err = execSQL(context.Background(), db, "test_empty.sql")
	if err == nil {
		t.Fatalf("expected error for empty SQL file, got nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("unexpected error message: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestExecSQL_Success(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_valid.sql")
	if err != nil {
		t.Fatalf("failed to read test_valid.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(1, 1))
	err = execSQL(context.Background(), db, "test_valid.sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestExecSQL_QueryExecutionError(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_fail.sql")
	if err != nil {
		t.Fatalf("failed to read test_fail.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	expectedErr := errors.New("execution error")

	mock.ExpectExec(query).WillReturnError(expectedErr)
	err = execSQL(context.Background(), db, "test_fail.sql")
	if err == nil {
		t.Fatalf("expected error from ExecContext, got nil")
	}
	if !strings.Contains(err.Error(), "SQL execution failed") {
		t.Fatalf("unexpected error message: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestExecSQLNoCtx_Success(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_valid.sql")
	if err != nil {
		t.Fatalf("failed to read test_valid.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	mock.ExpectExec(regexp.QuoteMeta(query)).WillReturnResult(sqlmock.NewResult(1, 1))
	err = execSQLNoCtx(db, "test_valid.sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

// -------------------
// SELECT 쿼리 관련 테스트 (querySQL, querySQLNoCtx)
// -------------------

func TestQuerySQL_FileNotFound(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	_, err = querySQL(context.Background(), db, "nonexistent.sql")
	if err == nil {
		t.Fatalf("expected error for nonexistent file, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read SQL file") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestQuerySQL_EmptyFile(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()

	_, err = querySQL(context.Background(), db, "test_empty.sql")
	if err == nil {
		t.Fatalf("expected error for empty SQL file, got nil")
	}
	if !strings.Contains(err.Error(), "is empty") {
		t.Fatalf("unexpected error message: %v", err)
	}
}

func TestQuerySQL_Success(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_select_valid.sql")
	if err != nil {
		t.Fatalf("failed to read test_select_valid.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	// 모의 결과 행 생성
	rows := sqlmock.NewRows([]string{"col"}).AddRow(1)

	mock.ExpectQuery(query).WillReturnRows(rows)
	result, err := querySQL(context.Background(), db, "test_select_valid.sql")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer func() {
		if cErr := result.Close(); cErr != nil {
			logger.Warnf("failed to close result: %v", cErr)
		}
	}()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestQuerySQL_QueryError(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_select_fail.sql")
	if err != nil {
		t.Fatalf("failed to read test_select_fail.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	expectedErr := errors.New("query error")

	mock.ExpectQuery(regexp.QuoteMeta(query)).WillReturnError(expectedErr)
	_, err = querySQL(context.Background(), db, "test_select_fail.sql")
	if err == nil {
		t.Fatalf("expected error from QueryContext, got nil")
	}
	if !strings.Contains(err.Error(), "SQL query failed") {
		t.Fatalf("unexpected error message: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("there were unfulfilled expectations: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

func TestQuerySQLNoCtx_Success(t *testing.T) {
	initTestFS(t)
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("failed to open sqlmock database: %v", err)
	}

	content, err := fs.ReadFile(sqlFiles, "queries/test_select_valid.sql")
	if err != nil {
		t.Fatalf("failed to read test_select_valid.sql: %v", err)
	}
	query := strings.TrimSpace(string(content))

	rows := sqlmock.NewRows([]string{"col"}).AddRow(1)

	mock.ExpectQuery(query).WillReturnRows(rows)
	result, err := querySQLNoCtx(db, "test_select_valid.sql")
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	defer func() {
		if cErr := result.Close(); cErr != nil {
			logger.Warnf("failed to close result: %v", cErr)
		}
	}()

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("there were unfulfilled expectations: %v", err)
	}

	mock.ExpectClose() // db.Close() 호출에 대한 기대 등록
	defer func() {
		cErr := db.Close()
		if cErr != nil {
			logger.Warnf("failed to db close: %v", cErr)
		}
	}()
}

// TestConnectDB_NonSQLite 드라이버가 sqlite 가 아니면 에러가 발생하는지 검증
func TestConnectDB_NonSQLite(t *testing.T) {
	// "sqlite3"가 아닌 드라이버("sqlmock")를 사용하면 에러를 반환해야 한다.
	_, err := ConnectDB("sqlmock", "dummy", true)
	if err == nil {
		t.Fatalf("expected error when using non-sqlite driver, got nil")
	}
	expected := "unsupported driver: sqlmock; only sqlite3 is supported"
	if err.Error() != expected {
		t.Fatalf("unexpected error message: got %q, want %q", err.Error(), expected)
	}
}
