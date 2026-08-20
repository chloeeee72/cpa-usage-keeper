package migration

import (
	"database/sql"
	"path/filepath"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddUsageEventErrorFieldsMigrationAddsColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "legacy.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer closeOpenedDatabase(t, db)

	for _, statement := range []string{
		`CREATE TABLE usage_events (
			id integer PRIMARY KEY,
			event_key text,
			model text,
			timestamp datetime,
			source text,
			auth_index text,
			failed integer,
			total_tokens integer
		)`,
		`INSERT INTO usage_events (id, event_key, model, timestamp, source, auth_index, failed, total_tokens)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		`CREATE TABLE usage_events_archive (
			id integer PRIMARY KEY,
			event_key text,
			model text,
			timestamp datetime,
			source text,
			auth_index text,
			failed integer,
			total_tokens integer
		)`,
		`INSERT INTO usage_events_archive (id, event_key, model, timestamp, source, auth_index, failed, total_tokens)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
	} {
		if statement == `INSERT INTO usage_events (id, event_key, model, timestamp, source, auth_index, failed, total_tokens)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)` {
			if err := db.Exec(statement, int64(1), "event-1", "claude-sonnet", "2026-08-20 08:00:00", "source-a", "auth-1", 1, 10).Error; err != nil {
				t.Fatalf("seed legacy usage event: %v", err)
			}
			continue
		}
		if statement == `INSERT INTO usage_events_archive (id, event_key, model, timestamp, source, auth_index, failed, total_tokens)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)` {
			if err := db.Exec(statement, int64(1), "event-1", "claude-sonnet", "2026-08-20 08:00:00", "source-a", "auth-1", 1, 10).Error; err != nil {
				t.Fatalf("seed legacy usage event archive: %v", err)
			}
			continue
		}
		if err := db.Exec(statement).Error; err != nil {
			t.Fatalf("prepare legacy schema with %q: %v", statement, err)
		}
	}

	if err := addUsageEventErrorFieldsMigration(db); err != nil {
		t.Fatalf("add usage event error fields: %v", err)
	}
	if err := addUsageEventErrorFieldsMigration(db); err != nil {
		t.Fatalf("add usage event error fields should be idempotent: %v", err)
	}

	for _, table := range []string{"usage_events", "usage_events_archive"} {
		for _, column := range []string{"error_code", "error_message"} {
			if !db.Migrator().HasColumn(table, column) {
				t.Fatalf("expected %s.%s column to exist", table, column)
			}
		}
	}

	for _, table := range []string{"usage_events", "usage_events_archive"} {
		var errorCode sql.NullString
		var errorMessage sql.NullString
		if err := db.Raw(`SELECT error_code, error_message FROM `+table+` WHERE id = ?`, int64(1)).Row().Scan(&errorCode, &errorMessage); err != nil {
			t.Fatalf("scan %s error fields: %v", table, err)
		}
		if errorCode.Valid {
			t.Fatalf("expected existing %s row error_code to stay NULL, got %q", table, errorCode.String)
		}
		if errorMessage.Valid {
			t.Fatalf("expected existing %s row error_message to stay NULL, got %q", table, errorMessage.String)
		}
	}
}

func TestAddUsageEventErrorFieldsMigrationSkipsMissingTables(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "empty.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer closeOpenedDatabase(t, db)

	if err := addUsageEventErrorFieldsMigration(db); err != nil {
		t.Fatalf("expected migration without usage_events tables to succeed, got %v", err)
	}
}
