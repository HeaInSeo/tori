package db

import (
	"context"
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"strings"

	globallog "github.com/seoyhaein/tori/log"
)

var (
	logger = globallog.Log
	//go:embed queries/*.sql
	embeddedFiles embed.FS
	sqlFiles      fs.FS = embeddedFiles
)

// execSQLTx 읽어온 SQL 파일을 트랜잭션 내에서 ExecContext 로 실행.
// IMPORTANT: 비 SELECT 쿼리에 사용. (결과 리턴 없음) 호출하는 쪽에서 트랜젝션의 commit 이나 rollback 을 신경써줘야 함.
func execSQLTx(ctx context.Context, tx *sql.Tx, fileName string, args ...interface{}) error {
	// "queries/" 하위의 SQL 파일을 읽어옴.
	content, err := fs.ReadFile(sqlFiles, "queries/"+fileName)
	if err != nil {
		return fmt.Errorf("failed to read SQL file (%s): %w", fileName, err)
	}

	query := strings.TrimSpace(string(content))
	if query == "" {
		return fmt.Errorf("SQL file (%s) is empty", fileName)
	}

	_, err = tx.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("SQL execution failed (%s): %w", fileName, err)
	}

	return nil
}

// execSQLTxNoCtx 컨텍스트 없이 트랜잭션 내에서 SQL 파일을 실행.
func execSQLTxNoCtx(tx *sql.Tx, fileName string, args ...interface{}) error {
	return execSQLTx(context.Background(), tx, fileName, args...)
}

// execSQL 읽어온 SQL 파일을 DB 에서 ExecContext 로 실행.
// IMPORTANT: 비 SELECT 쿼리에 사용. (결과 리턴 없음) 호출하는 쪽에서 트랜젝션의 commit 이나 rollback 을 신경써줘야 함.
func execSQL(ctx context.Context, db *sql.DB, fileName string, args ...interface{}) error {
	// "queries/" 하위의 SQL 파일을 읽어옴.
	content, err := fs.ReadFile(sqlFiles, "queries/"+fileName)
	if err != nil {
		return fmt.Errorf("failed to read SQL file (%s): %w", fileName, err)
	}

	query := strings.TrimSpace(string(content))
	if query == "" {
		return fmt.Errorf("SQL file (%s) is empty", fileName)
	}

	_, err = db.ExecContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("SQL execution failed (%s): %w", fileName, err)
	}

	return nil
}

// execSQLNoCtx 컨텍스트 없이 DB 에서 SQL 파일을 실행.
func execSQLNoCtx(db *sql.DB, fileName string, args ...interface{}) error {
	return execSQL(context.Background(), db, fileName, args...)
}

// querySQL 읽어온 SQL 파일을 DB 에서 QueryContext 로 실행.
// IMPORTANT: SELECT 쿼리에 사용. 결과로 *sql.Rows 를 반환하며, 호출자가 반드시 Close() 해야 함.  않하면 memory leak 발생.
func querySQL(ctx context.Context, db *sql.DB, fileName string, args ...interface{}) (*sql.Rows, error) {
	// "queries/" 하위의 SQL 파일을 읽어옴.
	content, err := fs.ReadFile(sqlFiles, "queries/"+fileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read SQL file (%s): %w", fileName, err)
	}

	query := strings.TrimSpace(string(content))
	if query == "" {
		return nil, fmt.Errorf("SQL file (%s) is empty", fileName)
	}

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("SQL query failed (%s): %w", fileName, err)
	}
	return rows, nil
}

// querySQLNoCtx 컨텍스트 없이 DB 에서 SELECT 쿼리를 실행.
// IMPORTANT: SELECT 쿼리에 사용. 결과로 *sql.Rows 를 반환하며, 호출자가 반드시 Close() 해야 함. 않하면 memory leak 발생.
func querySQLNoCtx(db *sql.DB, fileName string, args ...interface{}) (*sql.Rows, error) {
	return querySQL(context.Background(), db, fileName, args...)
}

// CheckForeignKeysEnabled DB 연결에서 외래 키가 활성화되었는지 확인함.
// IMPORTANT: fk 값이 1이면 외래 키가 활성화된 상태임.
func CheckForeignKeysEnabled(db *sql.DB) (bool, error) {
	var fk int
	err := db.QueryRow("PRAGMA foreign_keys").Scan(&fk)
	if err != nil {
		return false, fmt.Errorf("failed to check foreign keys: %w", err)
	}
	return fk == 1, nil
}
