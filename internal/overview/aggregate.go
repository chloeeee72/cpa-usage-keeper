package overview

import (
	"sort"
	"strings"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/timeutil"
)

type aggregateKey struct {
	BucketStart         time.Time
	APIGroupKey         string
	Model               string
	AuthIndex           string
	ModelAlias          string
	ServiceTier         string
	ResponseServiceTier string
	ReasoningEffort     string
	Endpoint            string
	ExecutorType        string
	PricingPeriod       string
}

// BuildRows 使用最终唯一键把一批 usage events 同时聚合成 hourly 和 daily rows。
func BuildRows(events []entities.UsageEvent) ([]entities.UsageOverviewHourlyStat, []entities.UsageOverviewDailyStat, int64) {
	return BuildRowsWithPeakHours(events, nil)
}

// BuildRowsWithPeakHours 使用高峰时段配置把每条事件拆分为 peak/off_peak 聚合行。
func BuildRowsWithPeakHours(events []entities.UsageEvent, peakHours *pricing.PeakHoursConfig) ([]entities.UsageOverviewHourlyStat, []entities.UsageOverviewDailyStat, int64) {
	// 两个 map 都直接使用数据库最终唯一键，迁移与运行时不会产生不同分组。
	hourly := make(map[aggregateKey]*entities.UsageOverviewHourlyStat)
	daily := make(map[aggregateKey]*entities.UsageOverviewDailyStat)
	maxEventID := int64(0)

	for _, event := range events {
		// checkpoint 只推进到当前 batch 实际构建完成的最大事件 ID。
		if event.ID > maxEventID {
			maxEventID = event.ID
		}
		// 所有字符串维度在进入唯一键前统一清理首尾空白。
		dimensions := aggregateKey{
			APIGroupKey:         normalizeRequiredDimension(event.APIGroupKey),
			Model:               normalizeRequiredDimension(event.Model),
			AuthIndex:           normalizeOptionalDimension(event.AuthIndex),
			ServiceTier:         normalizeOptionalDimension(event.ServiceTier),
			ResponseServiceTier: normalizeOptionalDimension(event.ResponseServiceTier),
			ReasoningEffort:     normalizeOptionalDimension(event.ReasoningEffort),
			Endpoint:            normalizeOptionalDimension(event.Endpoint),
			ExecutorType:        normalizeOptionalDimension(event.ExecutorType),
			PricingPeriod:       pricingPeriodForEvent(event.Timestamp, peakHours),
		}
		// nullable model_alias 与 nullable endpoint 一样归一为空字符串。
		if event.ModelAlias != nil {
			dimensions.ModelAlias = normalizeOptionalDimension(*event.ModelAlias)
		}

		// 时间先归一到项目存储时区，再沿用现有整点和本地自然日边界。
		timestamp := timeutil.NormalizeStorageTime(event.Timestamp)
		hourKey := dimensions
		hourKey.BucketStart = timestamp.Truncate(time.Hour)
		dayKey := dimensions
		dayKey.BucketStart = time.Date(timestamp.Year(), timestamp.Month(), timestamp.Day(), 0, 0, 0, 0, timestamp.Location())

		// 第一次遇到最终唯一键时创建维度完整的稀疏行。
		if hourly[hourKey] == nil {
			hourly[hourKey] = &entities.UsageOverviewHourlyStat{
				BucketStart: hourKey.BucketStart, APIGroupKey: hourKey.APIGroupKey, Model: hourKey.Model,
				AuthIndex: hourKey.AuthIndex, ModelAlias: hourKey.ModelAlias, ServiceTier: hourKey.ServiceTier,
				ResponseServiceTier: hourKey.ResponseServiceTier, ReasoningEffort: hourKey.ReasoningEffort,
				Endpoint: hourKey.Endpoint, ExecutorType: hourKey.ExecutorType, PricingPeriod: hourKey.PricingPeriod,
			}
		}
		if daily[dayKey] == nil {
			daily[dayKey] = &entities.UsageOverviewDailyStat{
				BucketStart: dayKey.BucketStart, APIGroupKey: dayKey.APIGroupKey, Model: dayKey.Model,
				AuthIndex: dayKey.AuthIndex, ModelAlias: dayKey.ModelAlias, ServiceTier: dayKey.ServiceTier,
				ResponseServiceTier: dayKey.ResponseServiceTier, ReasoningEffort: dayKey.ReasoningEffort,
				Endpoint: dayKey.Endpoint, ExecutorType: dayKey.ExecutorType, PricingPeriod: dayKey.PricingPeriod,
			}
		}
		addEventToHourlyRow(hourly[hourKey], event)
		addEventToDailyRow(daily[dayKey], event)
	}

	// map 转切片后固定写入顺序，让迁移重跑和故障定位保持稳定。
	hourlyRows := make([]entities.UsageOverviewHourlyStat, 0, len(hourly))
	for _, row := range hourly {
		hourlyRows = append(hourlyRows, *row)
	}
	dailyRows := make([]entities.UsageOverviewDailyStat, 0, len(daily))
	for _, row := range daily {
		dailyRows = append(dailyRows, *row)
	}
	sort.Slice(hourlyRows, func(left, right int) bool {
		return hourlyRowLess(hourlyRows[left], hourlyRows[right])
	})
	sort.Slice(dailyRows, func(left, right int) bool {
		return dailyRowLess(dailyRows[left], dailyRows[right])
	})
	return hourlyRows, dailyRows, maxEventID
}

func normalizeRequiredDimension(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "unknown"
	}
	return trimmed
}

func normalizeOptionalDimension(value string) string {
	return strings.TrimSpace(value)
}

func addEventToHourlyRow(row *entities.UsageOverviewHourlyStat, event entities.UsageEvent) {
	row.RequestCount++
	if event.Failed {
		row.FailureCount++
	} else {
		row.SuccessCount++
	}
	row.InputTokens += event.InputTokens
	row.OutputTokens += event.OutputTokens
	row.ReasoningTokens += event.ReasoningTokens
	row.CachedTokens += event.CachedTokens
	row.CacheReadTokens += event.CacheReadTokens
	row.CacheCreationTokens += event.CacheCreationTokens
	row.TotalTokens += event.TotalTokens
}

func addEventToDailyRow(row *entities.UsageOverviewDailyStat, event entities.UsageEvent) {
	row.RequestCount++
	if event.Failed {
		row.FailureCount++
	} else {
		row.SuccessCount++
	}
	row.InputTokens += event.InputTokens
	row.OutputTokens += event.OutputTokens
	row.ReasoningTokens += event.ReasoningTokens
	row.CachedTokens += event.CachedTokens
	row.CacheReadTokens += event.CacheReadTokens
	row.CacheCreationTokens += event.CacheCreationTokens
	row.TotalTokens += event.TotalTokens
}

func hourlyRowLess(left, right entities.UsageOverviewHourlyStat) bool {
	return dimensionsLess(
		aggregateKey{left.BucketStart, left.APIGroupKey, left.Model, left.AuthIndex, left.ModelAlias, left.ServiceTier, left.ResponseServiceTier, left.ReasoningEffort, left.Endpoint, left.ExecutorType, left.PricingPeriod},
		aggregateKey{right.BucketStart, right.APIGroupKey, right.Model, right.AuthIndex, right.ModelAlias, right.ServiceTier, right.ResponseServiceTier, right.ReasoningEffort, right.Endpoint, right.ExecutorType, right.PricingPeriod},
	)
}

func dailyRowLess(left, right entities.UsageOverviewDailyStat) bool {
	return dimensionsLess(
		aggregateKey{left.BucketStart, left.APIGroupKey, left.Model, left.AuthIndex, left.ModelAlias, left.ServiceTier, left.ResponseServiceTier, left.ReasoningEffort, left.Endpoint, left.ExecutorType, left.PricingPeriod},
		aggregateKey{right.BucketStart, right.APIGroupKey, right.Model, right.AuthIndex, right.ModelAlias, right.ServiceTier, right.ResponseServiceTier, right.ReasoningEffort, right.Endpoint, right.ExecutorType, right.PricingPeriod},
	)
}

func dimensionsLess(left, right aggregateKey) bool {
	if !left.BucketStart.Equal(right.BucketStart) {
		return left.BucketStart.Before(right.BucketStart)
	}
	leftValues := [...]string{left.APIGroupKey, left.Model, left.AuthIndex, left.ModelAlias, left.ServiceTier, left.ResponseServiceTier, left.ReasoningEffort, left.Endpoint, left.ExecutorType, left.PricingPeriod}
	rightValues := [...]string{right.APIGroupKey, right.Model, right.AuthIndex, right.ModelAlias, right.ServiceTier, right.ResponseServiceTier, right.ReasoningEffort, right.Endpoint, right.ExecutorType, right.PricingPeriod}
	for index := range leftValues {
		if leftValues[index] != rightValues[index] {
			return leftValues[index] < rightValues[index]
		}
	}
	return false
}

func pricingPeriodForEvent(timestamp time.Time, peakHours *pricing.PeakHoursConfig) string {
	if peakHours == nil || timestamp.IsZero() {
		return string(pricing.PricingPeriodPeak)
	}
	if peakHours.IsPeak(timestamp) {
		return string(pricing.PricingPeriodPeak)
	}
	return string(pricing.PricingPeriodOffPeak)
}
