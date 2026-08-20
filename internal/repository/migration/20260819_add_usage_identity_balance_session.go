package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

func addUsageIdentityBalanceSessionMigration(tx *gorm.DB) error {
	if !tx.Migrator().HasTable(&entities.UsageIdentity{}) || tx.Migrator().HasColumn(&entities.UsageIdentity{}, "balance_session") {
		return nil
	}
	if err := tx.Migrator().AddColumn(&entities.UsageIdentity{}, "BalanceSession"); err != nil {
		return fmt.Errorf("add usage_identities.balance_session column: %w", err)
	}
	return nil
}
