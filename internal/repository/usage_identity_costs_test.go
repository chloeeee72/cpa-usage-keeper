package repository

import (
	"context"
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
)

func TestAggregateUsageIdentityCostsUsesDailyBeforeTodayAndHourlyForToday(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previousLocal })

	db := openTestDatabase(t)
	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)

	dailyRows := []entities.UsageOverviewDailyStat{
		{
			BucketStart:         time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC),
			APIGroupKey:         "key-a",
			AuthIndex:           "auth-a",
			Model:               "daily-model",
			InputTokens:         1000,
			OutputTokens:        50,
			CacheReadTokens:     200,
			CacheCreationTokens: 100,
		},
		// 今天 00:00 的 daily 行不能进入聚合。
		{
			BucketStart: time.Date(2026, 8, 20, 0, 0, 0, 0, time.UTC),
			APIGroupKey: "key-a",
			AuthIndex:   "auth-a",
			Model:       "today-daily-excluded",
			InputTokens: 999,
		},
	}
	hourlyRows := []entities.UsageOverviewHourlyStat{
		{
			BucketStart:     time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC),
			APIGroupKey:     "key-a",
			AuthIndex:       "auth-a",
			Model:           "hourly-model",
			InputTokens:     400,
			OutputTokens:    10,
			CacheReadTokens: 50,
		},
		// 昨天的小时行不能进入聚合。
		{
			BucketStart: time.Date(2026, 8, 19, 9, 0, 0, 0, time.UTC),
			APIGroupKey: "key-a",
			AuthIndex:   "auth-a",
			Model:       "yesterday-hourly-excluded",
			InputTokens: 888,
		},
	}
	if err := db.Create(&dailyRows).Error; err != nil {
		t.Fatalf("seed daily stats: %v", err)
	}
	if err := db.Create(&hourlyRows).Error; err != nil {
		t.Fatalf("seed hourly stats: %v", err)
	}

	rows, err := AggregateUsageIdentityCosts(context.Background(), db, []string{"auth-a"}, pricing.ActiveFields(0), now)
	if err != nil {
		t.Fatalf("AggregateUsageIdentityCosts returned error: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected daily before today and hourly today rows only, got %d: %+v", len(rows), rows)
	}

	byModel := make(map[string]UsageIdentityCostAggregate, len(rows))
	for _, row := range rows {
		byModel[row.Model] = row
	}
	daily := byModel["daily-model"]
	if daily.CostUncachedInputTokens != 700 || daily.CostCacheReadTokens != 200 || daily.CostCacheCreationTokens != 100 || daily.CostOutputTokens != 50 {
		t.Fatalf("unexpected daily aggregate: %+v", daily)
	}
	hourly := byModel["hourly-model"]
	if hourly.CostUncachedInputTokens != 350 || hourly.CostCacheReadTokens != 50 || hourly.CostOutputTokens != 10 {
		t.Fatalf("unexpected hourly aggregate: %+v", hourly)
	}
	if _, ok := byModel["today-daily-excluded"]; ok {
		t.Fatalf("today daily row should be excluded: %+v", rows)
	}
	if _, ok := byModel["yesterday-hourly-excluded"]; ok {
		t.Fatalf("yesterday hourly row should be excluded: %+v", rows)
	}
}

func TestAggregateUsageIdentityCostsSkipsEmptyAuthIndexes(t *testing.T) {
	db := openTestDatabase(t)
	rows, err := AggregateUsageIdentityCosts(context.Background(), db, nil, pricing.ActiveFields(0), time.Now())
	if err != nil {
		t.Fatalf("AggregateUsageIdentityCosts returned error: %v", err)
	}
	if rows != nil {
		t.Fatalf("expected nil rows for empty auth indexes, got %+v", rows)
	}
}
