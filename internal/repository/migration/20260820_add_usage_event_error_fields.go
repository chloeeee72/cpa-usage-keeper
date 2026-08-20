package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

func addUsageEventErrorFieldsMigration(tx *gorm.DB) error {
	columns := []struct {
		name string
		sql  string
	}{
		{name: "error_code", sql: "ALTER TABLE usage_events ADD COLUMN error_code TEXT"},
		{name: "error_message", sql: "ALTER TABLE usage_events ADD COLUMN error_message TEXT"},
	}
	for _, column := range columns {
		if !tx.Migrator().HasTable(&entities.UsageEvent{}) || tx.Migrator().HasColumn(&entities.UsageEvent{}, column.name) {
			continue
		}
		if err := tx.Exec(column.sql).Error; err != nil {
			return fmt.Errorf("add usage_events.%s column: %w", column.name, err)
		}
	}
	if tx.Migrator().HasTable(&entities.UsageEventArchive{}) {
		for _, column := range columns {
			if tx.Migrator().HasColumn(&entities.UsageEventArchive{}, column.name) {
				continue
			}
			if err := tx.Exec("ALTER TABLE usage_events_archive ADD COLUMN " + column.name + " TEXT").Error; err != nil {
				return fmt.Errorf("add usage_events_archive.%s column: %w", column.name, err)
			}
		}
	}
	return nil
}
