package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/timeutil"

	"gorm.io/gorm"
)

type UsageIdentityCostAggregate struct {
	AuthIndex               string
	APIGroupKey             string
	Model                   string
	ModelAlias              string
	ServiceTier             string
	ResponseServiceTier     string
	ReasoningEffort         string
	Endpoint                string
	ExecutorType            string
	PricingPeriod           string
	RequestCount            int64
	SuccessCount            int64
	FailureCount            int64
	InputTokens             int64
	ReasoningTokens         int64
	CacheReadTokens         int64
	CacheCreationTokens     int64
	TotalTokens             int64
	CostUncachedInputTokens int64
	CostOutputTokens        int64
	CostCacheReadTokens     int64
	CostCacheCreationTokens int64
}

// AggregateUsageIdentityCosts 按 auth_index + 价格维度聚合全历史 rollup，用于按身份计算成本。
// 完整自然日读 daily stats，今天读 hourly stats，避免扫描整张小时表。
func AggregateUsageIdentityCosts(ctx context.Context, db *gorm.DB, authIndexes []string, activeFields pricing.ActiveFields, now time.Time) ([]UsageIdentityCostAggregate, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	authIndexes = normalizeUniqueAuthIndexes(authIndexes)
	if len(authIndexes) == 0 {
		return nil, nil
	}
	localNow := timeutil.NormalizeStorageTime(now)
	todayStart := time.Date(localNow.Year(), localNow.Month(), localNow.Day(), 0, 0, 0, 0, time.Local)
	todayStartText := timeutil.FormatStorageTime(todayStart)

	dimensions := append([]string{"auth_index"}, UsagePricingDimensionColumns(activeFields)...)
	selectColumns := strings.Join(dimensions, ", ") + ", " + usageOverviewStatProjectionAggregateColumns
	groupColumns := strings.Join(dimensions, ", ")

	var rows []UsageIdentityCostAggregate
	dailyQuery := db.WithContext(ctx).
		Model(&entities.UsageOverviewDailyStat{}).
		Select(selectColumns).
		Where("auth_index IN ? AND bucket_start < ?", authIndexes, todayStartText).
		Group(groupColumns)
	if err := dailyQuery.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("aggregate usage identity daily costs: %w", err)
	}

	var hourlyRows []UsageIdentityCostAggregate
	hourlyQuery := db.WithContext(ctx).
		Model(&entities.UsageOverviewHourlyStat{}).
		Select(selectColumns).
		Where("auth_index IN ? AND bucket_start >= ?", authIndexes, todayStartText).
		Group(groupColumns)
	if err := hourlyQuery.Scan(&hourlyRows).Error; err != nil {
		return nil, fmt.Errorf("aggregate usage identity hourly costs: %w", err)
	}
	rows = append(rows, hourlyRows...)
	return rows, nil
}
