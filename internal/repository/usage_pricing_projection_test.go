package repository

import (
	"strings"
	"testing"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
)

func TestUsagePricingProjectionColumnsSkipsPricingPeriod(t *testing.T) {
	multiplier := 1.0
	snapshot, err := pricing.CompileSnapshot([]pricing.ModelConfig{{
		Pricing: entities.ModelPriceSetting{Model: "model-a", PromptPricePer1M: 1, PriceMultiplier: &multiplier},
		Rules: []pricing.RuleConfig{
			{Key: "pricing_period", Value: "peak", Multiplier: 2},
			{Key: "service_tier", Value: "priority", Multiplier: 3},
		},
	}})
	if err != nil {
		t.Fatalf("CompileSnapshot: %v", err)
	}
	resolver := pricing.NewCatalog(snapshot).NewResolver()
	projection := usagePricingProjectionColumns(usageOverviewBoundaryEventProjectionColumns, resolver.ActiveFields())
	if strings.Contains(projection, "pricing_period") {
		t.Fatalf("usage_events projection must not include pricing_period: %s", projection)
	}
	if !strings.Contains(projection, "service_tier") {
		t.Fatalf("expected active service_tier dimension in projection: %s", projection)
	}
}
