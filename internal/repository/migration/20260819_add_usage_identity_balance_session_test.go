package migration

import (
	"database/sql"
	"path/filepath"
	"testing"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestAddUsageIdentityBalanceSessionMigrationAddsNullableColumn(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(testSQLiteDSN(filepath.Join(t.TempDir(), "legacy.db"))), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database: %v", err)
	}
	defer closeOpenedDatabase(t, db)

	if err := db.Exec(`CREATE TABLE usage_identities (
		id integer PRIMARY KEY AUTOINCREMENT,
		name text,
		auth_type integer,
		auth_type_name text,
		identity text,
		type text,
		provider text,
		lookup_key text,
		prefix text,
		base_url text,
		is_deleted numeric
	)`).Error; err != nil {
		t.Fatalf("create legacy usage_identities table: %v", err)
	}
	if err := db.Exec(`INSERT INTO usage_identities (name, auth_type, auth_type_name, identity, type, provider, lookup_key, prefix, base_url, is_deleted)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, "TokenRhythm", entities.UsageIdentityAuthTypeAIProvider, "apikey", "tr-auth", "openai", "TokenRhythm", "tr-key", "tr", "https://tokenrhythm.studio/v1", false).Error; err != nil {
		t.Fatalf("seed legacy usage identity: %v", err)
	}

	if err := addUsageIdentityBalanceSessionMigration(db); err != nil {
		t.Fatalf("add usage identity balance_session: %v", err)
	}
	if err := addUsageIdentityBalanceSessionMigration(db); err != nil {
		t.Fatalf("add usage identity balance_session should be idempotent: %v", err)
	}
	if !db.Migrator().HasColumn(&entities.UsageIdentity{}, "balance_session") {
		t.Fatal("expected usage_identities.balance_session column to exist")
	}

	var session sql.NullString
	err = db.Raw(`SELECT balance_session FROM usage_identities WHERE identity = ?`, "tr-auth").Row().Scan(&session)
	if err != nil {
		t.Fatalf("scan balance_session: %v", err)
	}
	if session.Valid {
		t.Fatalf("expected legacy balance_session to default NULL, got %+v", session)
	}
}
