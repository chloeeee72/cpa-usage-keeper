package service

import (
	"context"
	"math"
	"path/filepath"
	"testing"
	"time"

	"cpa-usage-keeper/internal/config"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"
	repodto "cpa-usage-keeper/internal/repository/dto"
)

func TestUsageIdentityCostServiceAggregatesAndPricesHistory(t *testing.T) {
	previousLocal := time.Local
	time.Local = time.UTC
	t.Cleanup(func() { time.Local = previousLocal })

	db, err := repository.OpenDatabase(config.Config{SQLitePath: filepath.Join(t.TempDir(), "usage-identity-cost-service.db")})
	if err != nil {
		t.Fatalf("OpenDatabase returned error: %v", err)
	}
	closeTestDatabase(t, db)

	now := time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC)
	if err := db.Create(&[]entities.UsageIdentity{
		{Name: "Priced Provider", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "auth-priced", Type: "openai", Provider: "OpenAI", CreatedAt: now, UpdatedAt: now},
		{Name: "Unpriced Provider", AuthType: entities.UsageIdentityAuthTypeAIProvider, AuthTypeName: "apikey", Identity: "auth-unpriced", Type: "openai", Provider: "OpenAI", CreatedAt: now, UpdatedAt: now},
	}).Error; err != nil {
		t.Fatalf("seed usage identities: %v", err)
	}
	if _, err := repository.UpsertModelPriceSetting(db, repodto.ModelPriceSettingInput{
		Model:                "claude-sonnet",
		PromptPricePer1M:     3,
		CompletionPricePer1M: 15,
		CacheReadPricePer1M:  0.3,
		CacheWritePricePer1M: 0.1,
	}); err != nil {
		t.Fatalf("UpsertModelPriceSetting returned error: %v", err)
	}
	if err := db.Create(&[]entities.UsageOverviewDailyStat{
		{
			BucketStart:         time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC),
			APIGroupKey:         "key-priced",
			AuthIndex:           "auth-priced",
			Model:               "claude-sonnet",
			InputTokens:         1000,
			OutputTokens:        50,
			CacheReadTokens:     200,
			CacheCreationTokens: 100,
		},
		{
			BucketStart: time.Date(2026, 8, 19, 0, 0, 0, 0, time.UTC),
			APIGroupKey: "key-unpriced",
			AuthIndex:   "auth-unpriced",
			Model:       "unpriced-model",
			InputTokens: 500,
		},
	}).Error; err != nil {
		t.Fatalf("seed daily stats: %v", err)
	}

	snapshot, err := repository.LoadPricingSnapshot(context.Background(), db)
	if err != nil {
		t.Fatalf("LoadPricingSnapshot returned error: %v", err)
	}
	service := &usageIdentityCostService{
		db:      db,
		catalog: pricing.NewCatalog(snapshot),
		now:     func() time.Time { return now },
	}

	response, err := service.ListUsageIdentityCosts(context.Background(), entities.UsageIdentityAuthTypeAIProvider)
	if err != nil {
		t.Fatalf("ListUsageIdentityCosts returned error: %v", err)
	}
	if len(response.Items) != 2 {
		t.Fatalf("expected two identity cost items, got %d: %+v", len(response.Items), response.Items)
	}

	byID := make(map[string]UsageIdentityCostItem, len(response.Items))
	for _, item := range response.Items {
		byID[item.IdentityID] = item
	}

	priced, ok := byID["1"]
	if !ok {
		t.Fatalf("expected cost item for identity_id 1, got %+v", response.Items)
	}
	unpriced, ok := byID["2"]
	if !ok {
		t.Fatalf("expected cost item for identity_id 2, got %+v", response.Items)
	}

	wantPriced := 700.0/1_000_000*3 + 200.0/1_000_000*0.3 + 100.0/1_000_000*0.1 + 50.0/1_000_000*15
	if !priced.CostAvailable {
		t.Fatalf("expected priced identity cost available, got %+v", priced)
	}
	if math.Abs(priced.TotalCostUSD-wantPriced) > 1e-9 {
		t.Fatalf("expected priced total cost %f, got %f", wantPriced, priced.TotalCostUSD)
	}
	if unpriced.CostAvailable {
		t.Fatalf("expected unpriced identity cost unavailable, got %+v", unpriced)
	}
	if unpriced.TotalCostUSD != 0 {
		t.Fatalf("expected unpriced identity total cost 0, got %f", unpriced.TotalCostUSD)
	}
}
