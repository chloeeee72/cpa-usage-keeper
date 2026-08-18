package migration

import (
	"fmt"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/overview"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository/overviewstore"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
)

const (
	usageOverviewPricingPeriodBatchSize = 1000
	peakHoursSettingKey                 = "pricing.peak_hours"
)

// usageOverviewPricingPeriodMigration 为 hourly/daily rollup 增加 pricing_period 维度并重建。
func usageOverviewPricingPeriodMigration(db *gorm.DB) error {
	now := timeutil.NormalizeStorageTime(time.Now())
	var targetEventID int64
	if db.Migrator().HasTable(&entities.UsageEvent{}) {
		if err := db.Model(&entities.UsageEvent{}).Select("COALESCE(MAX(id), 0)").Scan(&targetEventID).Error; err != nil {
			return fmt.Errorf("load usage overview pricing period target: %w", err)
		}
	}

	if err := prepareUsageOverviewPricingPeriod(db); err != nil {
		return err
	}

	peakHours, err := loadMigrationPeakHours(db)
	if err != nil {
		return err
	}

	for {
		processed, err := migrateUsageOverviewPricingPeriodBatch(db, now, targetEventID, peakHours)
		if err != nil {
			return err
		}
		if processed == 0 {
			break
		}
	}
	return verifyUsageOverviewPricingPeriodTarget(db, targetEventID)
}

func prepareUsageOverviewPricingPeriod(db *gorm.DB) error {
	return db.Transaction(func(tx *gorm.DB) error {
		for _, table := range []struct {
			model any
			table string
		}{
			{model: &entities.UsageOverviewHourlyStat{}, table: "usage_overview_hourly_stats"},
			{model: &entities.UsageOverviewDailyStat{}, table: "usage_overview_daily_stats"},
		} {
			if !tx.Migrator().HasColumn(table.model, "pricing_period") {
				if err := tx.Exec("ALTER TABLE " + table.table + " ADD COLUMN pricing_period TEXT NOT NULL DEFAULT 'peak'").Error; err != nil {
					return fmt.Errorf("add %s.pricing_period column: %w", table.table, err)
				}
			}
		}
		if tx.Migrator().HasIndex(&entities.UsageOverviewHourlyStat{}, "uniq_usage_overview_hourly_stats_dimensions") {
			if err := tx.Migrator().DropIndex(&entities.UsageOverviewHourlyStat{}, "uniq_usage_overview_hourly_stats_dimensions"); err != nil {
				return fmt.Errorf("drop hourly pricing period index: %w", err)
			}
		}
		if tx.Migrator().HasIndex(&entities.UsageOverviewDailyStat{}, "uniq_usage_overview_daily_stats_dimensions") {
			if err := tx.Migrator().DropIndex(&entities.UsageOverviewDailyStat{}, "uniq_usage_overview_daily_stats_dimensions"); err != nil {
				return fmt.Errorf("drop daily pricing period index: %w", err)
			}
		}

		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&entities.UsageOverviewHourlyStat{}).Error; err != nil {
			return fmt.Errorf("clear usage overview hourly stats: %w", err)
		}
		if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&entities.UsageOverviewDailyStat{}).Error; err != nil {
			return fmt.Errorf("clear usage overview daily stats: %w", err)
		}

		checkpoint := entities.UsageAggregationCheckpoint{Name: entities.UsageAggregationCheckpointOverview}
		if err := tx.Where("name = ?", entities.UsageAggregationCheckpointOverview).FirstOrCreate(&checkpoint).Error; err != nil {
			return fmt.Errorf("get usage overview pricing period checkpoint: %w", err)
		}
		if err := tx.Model(&entities.UsageAggregationCheckpoint{}).
			Where("name = ?", entities.UsageAggregationCheckpointOverview).
			Updates(map[string]any{
				"last_aggregated_usage_event_id": 0,
				"stats_updated_at":               nil,
			}).Error; err != nil {
			return fmt.Errorf("reset usage overview pricing period checkpoint: %w", err)
		}

		if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uniq_usage_overview_hourly_stats_dimensions ON usage_overview_hourly_stats (bucket_start, api_group_key, model, auth_index, model_alias, service_tier, response_service_tier, reasoning_effort, endpoint, executor_type, pricing_period)").Error; err != nil {
			return fmt.Errorf("create hourly pricing period index: %w", err)
		}
		if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS uniq_usage_overview_daily_stats_dimensions ON usage_overview_daily_stats (bucket_start, api_group_key, model, auth_index, model_alias, service_tier, response_service_tier, reasoning_effort, endpoint, executor_type, pricing_period)").Error; err != nil {
			return fmt.Errorf("create daily pricing period index: %w", err)
		}
		return nil
	})
}

func migrateUsageOverviewPricingPeriodBatch(db *gorm.DB, now time.Time, targetEventID int64, peakHours *pricing.PeakHoursConfig) (int, error) {
	processed := 0
	err := db.Transaction(func(tx *gorm.DB) error {
		var checkpoint entities.UsageAggregationCheckpoint
		if err := tx.Where("name = ?", entities.UsageAggregationCheckpointOverview).Take(&checkpoint).Error; err != nil {
			return fmt.Errorf("load usage overview pricing period checkpoint: %w", err)
		}
		if checkpoint.LastAggregatedUsageEventID >= targetEventID {
			return nil
		}
		var events []entities.UsageEvent
		if err := tx.Select("id, api_group_key, model, model_alias, auth_index, service_tier, response_service_tier, reasoning_effort, endpoint, executor_type, timestamp, failed, input_tokens, output_tokens, reasoning_tokens, cached_tokens, cache_read_tokens, cache_creation_tokens, total_tokens").
			Where("id > ? AND id <= ?", checkpoint.LastAggregatedUsageEventID, targetEventID).
			Order("id asc").
			Limit(usageOverviewPricingPeriodBatchSize).
			Find(&events).Error; err != nil {
			return fmt.Errorf("load usage overview pricing period events: %w", err)
		}
		if len(events) == 0 {
			return nil
		}
		hourlyRows, dailyRows, maxEventID := overview.BuildRowsWithPeakHours(events, peakHours)
		if err := overviewstore.ApplyRows(tx, hourlyRows, dailyRows, now); err != nil {
			return err
		}
		if err := tx.Model(&entities.UsageAggregationCheckpoint{}).
			Where("name = ?", entities.UsageAggregationCheckpointOverview).
			Updates(map[string]any{
				"last_aggregated_usage_event_id": maxEventID,
				"stats_updated_at":               timeutil.FormatStorageTime(now),
			}).Error; err != nil {
			return fmt.Errorf("update usage overview pricing period checkpoint: %w", err)
		}
		processed = len(events)
		return nil
	})
	return processed, err
}

func verifyUsageOverviewPricingPeriodTarget(db *gorm.DB, targetEventID int64) error {
	var checkpoint entities.UsageAggregationCheckpoint
	if err := db.Where("name = ?", entities.UsageAggregationCheckpointOverview).Take(&checkpoint).Error; err != nil {
		return fmt.Errorf("verify usage overview pricing period checkpoint: %w", err)
	}
	if checkpoint.LastAggregatedUsageEventID < targetEventID {
		return fmt.Errorf("usage overview pricing period checkpoint %d did not reach target %d", checkpoint.LastAggregatedUsageEventID, targetEventID)
	}
	return nil
}

func loadMigrationPeakHours(db *gorm.DB) (*pricing.PeakHoursConfig, error) {
	var setting entities.AppSetting
	if err := db.Where("setting_key = ?", peakHoursSettingKey).Take(&setting).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("load migration peak hours setting: %w", err)
	}
	if setting.Value == nil {
		return nil, nil
	}
	config, err := pricing.ParsePeakHoursConfig([]byte(*setting.Value))
	if err != nil {
		return nil, err
	}
	return config, nil
}
