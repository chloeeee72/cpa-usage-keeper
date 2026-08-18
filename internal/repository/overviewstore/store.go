package overviewstore

import (
	"fmt"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
)

const (
	overviewDimensionsPredicateLegacy       = "bucket_start = ? AND api_group_key = ? AND model = ? AND auth_index = ? AND model_alias = ? AND service_tier = ? AND response_service_tier = ? AND reasoning_effort = ? AND endpoint = ? AND executor_type = ?"
	overviewDimensionsPredicateWithPeriod = overviewDimensionsPredicateLegacy + " AND pricing_period = ?"
)

// ApplyRows 用同一套五维唯一键把 hourly 和 daily 增量写入当前事务。
func ApplyRows(tx *gorm.DB, hourlyRows []entities.UsageOverviewHourlyStat, dailyRows []entities.UsageOverviewDailyStat, now time.Time) error {
	// hourly 必须全部成功，调用方才会继续提交 daily 和 checkpoint。
	for _, row := range hourlyRows {
		if err := applyHourlyRow(tx, row, now); err != nil {
			return err
		}
	}
	// daily 与 hourly 共享同一事务和累计公式，禁止出现单表已推进状态。
	for _, row := range dailyRows {
		if err := applyDailyRow(tx, row, now); err != nil {
			return err
		}
	}
	return nil
}

func applyHourlyRow(tx *gorm.DB, row entities.UsageOverviewHourlyStat, now time.Time) error {
	row.PricingPeriod = normalizePricingPeriod(row.PricingPeriod)
	updates := tokenStatUpdates(row.RequestCount, row.SuccessCount, row.FailureCount, row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CachedTokens, row.CacheReadTokens, row.CacheCreationTokens, row.TotalTokens, now)
	predicate, args := overviewRowPredicate(tx, hourlyDimensionArgs(row), row.PricingPeriod)
	// update-first 避免正常累计走唯一索引冲突路径并消耗自增 ID。
	result := tx.Model(&entities.UsageOverviewHourlyStat{}).Where(predicate, args...).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update usage overview hourly stat: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	row.CreatedAt = timeutil.NormalizeStorageTime(now)
	row.UpdatedAt = timeutil.NormalizeStorageTime(now)
	create := tx.Create(&row)
	if !tx.Migrator().HasColumn(&entities.UsageOverviewHourlyStat{}, "pricing_period") {
		create = tx.Omit("pricing_period").Create(&row)
	}
	if insertErr := create.Error; insertErr != nil {
		// 并发创建相同 key 时只重试一次完整五维 UPDATE。
		retryResult := tx.Model(&entities.UsageOverviewHourlyStat{}).Where(predicate, args...).Updates(updates)
		if retryResult.Error != nil {
			return fmt.Errorf("insert usage overview hourly stat: %w; retry update: %v", insertErr, retryResult.Error)
		}
		if retryResult.RowsAffected == 0 {
			return fmt.Errorf("insert usage overview hourly stat: %w; retry update matched no existing row", insertErr)
		}
	}
	return nil
}

func applyDailyRow(tx *gorm.DB, row entities.UsageOverviewDailyStat, now time.Time) error {
	row.PricingPeriod = normalizePricingPeriod(row.PricingPeriod)
	updates := tokenStatUpdates(row.RequestCount, row.SuccessCount, row.FailureCount, row.InputTokens, row.OutputTokens, row.ReasoningTokens, row.CachedTokens, row.CacheReadTokens, row.CacheCreationTokens, row.TotalTokens, now)
	predicate, args := overviewRowPredicate(tx, dailyDimensionArgs(row), row.PricingPeriod)
	// daily 使用与 hourly 完全相同的最终唯一键和 update-first 语义。
	result := tx.Model(&entities.UsageOverviewDailyStat{}).Where(predicate, args...).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("update usage overview daily stat: %w", result.Error)
	}
	if result.RowsAffected > 0 {
		return nil
	}

	row.CreatedAt = timeutil.NormalizeStorageTime(now)
	row.UpdatedAt = timeutil.NormalizeStorageTime(now)
	create := tx.Create(&row)
	if !tx.Migrator().HasColumn(&entities.UsageOverviewDailyStat{}, "pricing_period") {
		create = tx.Omit("pricing_period").Create(&row)
	}
	if insertErr := create.Error; insertErr != nil {
		retryResult := tx.Model(&entities.UsageOverviewDailyStat{}).Where(predicate, args...).Updates(updates)
		if retryResult.Error != nil {
			return fmt.Errorf("insert usage overview daily stat: %w; retry update: %v", insertErr, retryResult.Error)
		}
		if retryResult.RowsAffected == 0 {
			return fmt.Errorf("insert usage overview daily stat: %w; retry update matched no existing row", insertErr)
		}
	}
	return nil
}

func hourlyDimensionArgs(row entities.UsageOverviewHourlyStat) []any {
	return []any{
		timeutil.FormatStorageTime(row.BucketStart), row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias,
		row.ServiceTier, row.ResponseServiceTier, row.ReasoningEffort, row.Endpoint, row.ExecutorType, row.PricingPeriod,
	}
}

func dailyDimensionArgs(row entities.UsageOverviewDailyStat) []any {
	return []any{
		timeutil.FormatStorageTime(row.BucketStart), row.APIGroupKey, row.Model, row.AuthIndex, row.ModelAlias,
		row.ServiceTier, row.ResponseServiceTier, row.ReasoningEffort, row.Endpoint, row.ExecutorType, row.PricingPeriod,
	}
}

func overviewRowPredicate(tx *gorm.DB, legacyArgs []any, period string) (string, []any) {
	if tx.Migrator().HasColumn(&entities.UsageOverviewHourlyStat{}, "pricing_period") {
		return overviewDimensionsPredicateWithPeriod, append(legacyArgs, normalizePricingPeriod(period))
	}
	return overviewDimensionsPredicateLegacy, legacyArgs
}

func normalizePricingPeriod(period string) string {
	if strings.TrimSpace(period) == "" {
		return string(pricing.PricingPeriodPeak)
	}
	return strings.TrimSpace(period)
}

func tokenStatUpdates(requestCount, successCount, failureCount, inputTokens, outputTokens, reasoningTokens, cachedTokens, cacheReadTokens, cacheCreationTokens, totalTokens int64, now time.Time) map[string]any {
	return map[string]any{
		"request_count":         gorm.Expr("request_count + ?", requestCount),
		"success_count":         gorm.Expr("success_count + ?", successCount),
		"failure_count":         gorm.Expr("failure_count + ?", failureCount),
		"input_tokens":          gorm.Expr("input_tokens + ?", inputTokens),
		"output_tokens":         gorm.Expr("output_tokens + ?", outputTokens),
		"reasoning_tokens":      gorm.Expr("reasoning_tokens + ?", reasoningTokens),
		"cached_tokens":         gorm.Expr("cached_tokens + ?", cachedTokens),
		"cache_read_tokens":     gorm.Expr("cache_read_tokens + ?", cacheReadTokens),
		"cache_creation_tokens": gorm.Expr("cache_creation_tokens + ?", cacheCreationTokens),
		"total_tokens":          gorm.Expr("total_tokens + ?", totalTokens),
		"updated_at":            timeutil.FormatStorageTime(now),
	}
}
