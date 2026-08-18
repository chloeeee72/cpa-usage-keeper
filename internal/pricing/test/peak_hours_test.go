package test

import (
	"testing"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
)

func TestDefaultPeakHoursConfigBoundaries(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	config := pricing.DefaultPeakHoursConfig()
	normalized, err := config.Normalize()
	if err != nil {
		t.Fatalf("normalize default peak hours: %v", err)
	}

	for _, testCase := range []struct {
		name  string
		clock string
		want  bool
	}{
		{name: "just before morning peak", clock: "08:59", want: false},
		{name: "morning peak start inclusive", clock: "09:00", want: true},
		{name: "morning peak inside", clock: "11:59", want: true},
		{name: "morning peak end exclusive", clock: "12:00", want: false},
		{name: "afternoon peak start inclusive", clock: "14:00", want: true},
		{name: "afternoon peak inside", clock: "17:59", want: true},
		{name: "afternoon peak end exclusive", clock: "18:00", want: false},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := time.ParseInLocation("15:04", testCase.clock, location)
			if err != nil {
				t.Fatalf("parse clock: %v", err)
			}
			instant := time.Date(2026, 8, 18, parsed.Hour(), parsed.Minute(), 0, 0, location)
			if got := normalized.IsPeak(instant); got != testCase.want {
				t.Fatalf("IsPeak(%s) = %v, want %v", testCase.clock, got, testCase.want)
			}
		})
	}
}

func TestPeakHoursConfigCrossMidnight(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	config := &pricing.PeakHoursConfig{
		Timezone: "Asia/Shanghai",
		Ranges: []pricing.PeakTimeRange{
			{Start: "22:00", End: "02:00"},
		},
	}
	normalized, err := config.Normalize()
	if err != nil {
		t.Fatalf("normalize cross midnight: %v", err)
	}
	for _, testCase := range []struct {
		name  string
		clock string
		want  bool
	}{
		{name: "before start", clock: "21:59", want: false},
		{name: "start inclusive", clock: "22:00", want: true},
		{name: "after midnight", clock: "01:59", want: true},
		{name: "end exclusive", clock: "02:00", want: false},
	} {
		testCase := testCase
		t.Run(testCase.name, func(t *testing.T) {
			t.Parallel()
			parsed, err := time.ParseInLocation("15:04", testCase.clock, location)
			if err != nil {
				t.Fatalf("parse clock: %v", err)
			}
			instant := time.Date(2026, 8, 18, parsed.Hour(), parsed.Minute(), 0, 0, location)
			if got := normalized.IsPeak(instant); got != testCase.want {
				t.Fatalf("IsPeak(%s) = %v, want %v", testCase.clock, got, testCase.want)
			}
		})
	}
}

func TestParsePeakHoursConfigRejectsInvalidInput(t *testing.T) {
	t.Parallel()

	for _, input := range []string{
		`{"timezone":"Asia/Shanghai","ranges":[{"start":"25:00","end":"12:00"}]}`,
		`{"timezone":"Not/AZone","ranges":[]}`,
		`{"timezone":"Asia/Shanghai","ranges":[{"start":"09:00","end":"09:00"}]}`,
	} {
		if _, err := pricing.ParsePeakHoursConfig([]byte(input)); err == nil {
			t.Fatalf("expected ParsePeakHoursConfig(%s) to fail", input)
		}
	}
}

func TestResolverAppliesPricingPeriodRule(t *testing.T) {
	t.Parallel()

	location, err := time.LoadLocation("Asia/Shanghai")
	if err != nil {
		t.Fatalf("load location: %v", err)
	}
	peakHours := pricing.DefaultPeakHoursConfig()
	normalized, err := peakHours.Normalize()
	if err != nil {
		t.Fatalf("normalize peak hours: %v", err)
	}
	setting := entities.ModelPriceSetting{
		Model:                "model-a",
		PricingStyle:         entities.ModelPricingStyleOpenAI,
		PromptPricePer1M:     10,
		CompletionPricePer1M: 0,
		CacheReadPricePer1M:  0,
		CacheWritePricePer1M: 0,
	}
	snapshot, err := pricing.CompileSnapshotWithPeakHours([]pricing.ModelConfig{
		{
			Pricing: setting,
			Rules: []pricing.RuleConfig{
				{Key: "pricing_period", Value: "off_peak", Multiplier: 0.5},
			},
		},
	}, normalized)
	if err != nil {
		t.Fatalf("compile snapshot with peak hours: %v", err)
	}
	resolver := pricing.NewCatalog(snapshot).NewResolver()

	peakInstant := time.Date(2026, 8, 18, 10, 0, 0, 0, location)
	offPeakInstant := time.Date(2026, 8, 18, 13, 0, 0, 0, location)
	tokens := helper.UsageTokenCostInput{InputTokens: 1_000_000}

	peakResult := resolver.Calculate(pricing.NewCostSubjectWithTimestamp(pricing.UsageDimensions{Model: "model-a"}, tokens, peakInstant))
	assertResultCost(t, peakResult, 10)
	if peakResult.RuleMultiplier != 1 {
		t.Fatalf("expected peak rule multiplier 1, got %+v", peakResult)
	}

	offPeakResult := resolver.Calculate(pricing.NewCostSubjectWithTimestamp(pricing.UsageDimensions{Model: "model-a"}, tokens, offPeakInstant))
	assertResultCost(t, offPeakResult, 5)
	if offPeakResult.RuleMultiplier != 0.5 {
		t.Fatalf("expected off-peak rule multiplier 0.5, got %+v", offPeakResult)
	}
}
