package service

import (
	"testing"

	"cpa-usage-keeper/internal/balance"
	"cpa-usage-keeper/internal/entities"
)

func TestSupportsTokenRhythmBalance(t *testing.T) {
	cases := []struct {
		name     string
		baseURL  string
		expected bool
	}{
		{name: "exact host", baseURL: "https://tokenrhythm.studio/v1", expected: true},
		{name: "exact host with port", baseURL: "https://tokenrhythm.studio:443/api", expected: true},
		{name: "subdomain", baseURL: "https://api.tokenrhythm.studio/v1", expected: true},
		{name: "unrelated", baseURL: "https://openrouter.ai/api/v1", expected: false},
		{name: "empty", baseURL: "", expected: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := SupportsTokenRhythmBalance(entities.UsageIdentity{BaseURL: tc.baseURL})
			if got != tc.expected {
				t.Fatalf("expected %v for base_url %q, got %v", tc.expected, tc.baseURL, got)
			}
		})
	}
}

func TestBuildBalanceQueryResponseTotals(t *testing.T) {
	items := []BalanceQueryItem{
		{IdentityID: "1", Summary: &balance.UsageSummary{BalanceCny: 10, AvailableBalanceCny: 6, FrozenBalanceCny: 4, ExpiringBalanceCny: 1, Calls: 5, CostCny: 2}},
		{IdentityID: "2", Summary: &balance.UsageSummary{BalanceCny: 20, AvailableBalanceCny: 20, Calls: 7, CostCny: 3}},
		{IdentityID: "3", Error: "failed"},
	}
	response := buildBalanceQueryResponse(items)
	if response.ConfiguredCount != 3 || response.SucceededCount != 2 || response.FailedCount != 1 {
		t.Fatalf("unexpected counts: %+v", response)
	}
	if response.Totals.BalanceCny != 30 || response.Totals.Calls != 12 || response.Totals.CostCny != 5 {
		t.Fatalf("unexpected totals: %+v", response.Totals)
	}
}

func TestValidateBalanceSession(t *testing.T) {
	if err := validateBalanceSession("valid-session-value"); err != nil {
		t.Fatalf("expected valid session, got %v", err)
	}
	if err := validateBalanceSession("bad\nsession"); err != ErrBalanceSessionInvalid {
		t.Fatalf("expected ErrBalanceSessionInvalid for newline, got %v", err)
	}
	if err := validateBalanceSession("bad\tsession"); err != ErrBalanceSessionInvalid {
		t.Fatalf("expected ErrBalanceSessionInvalid for tab, got %v", err)
	}
}
