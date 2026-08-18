package migration

import (
	"fmt"

	"cpa-usage-keeper/internal/entities"

	"gorm.io/gorm"
)

// modelPricePeakHoursConfigMigration 为模型价格表增加模型级高峰时段配置列。
func modelPricePeakHoursConfigMigration(db *gorm.DB) error {
	if !db.Migrator().HasColumn(&entities.ModelPriceSetting{}, "peak_hours_config") {
		if err := db.Exec("ALTER TABLE model_price_settings ADD COLUMN peak_hours_config TEXT").Error; err != nil {
			return fmt.Errorf("add model_price_settings.peak_hours_config column: %w", err)
		}
	}
	return nil
}
