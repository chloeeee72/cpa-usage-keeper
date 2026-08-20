package test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	. "cpa-usage-keeper/internal/api"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"
)

type usageIdentityCostProviderStub struct {
	response service.UsageIdentityCostsResponse
	err      error
	authType entities.UsageIdentityAuthType
	calls    int
}

func (s *usageIdentityCostProviderStub) ListUsageIdentityCosts(_ context.Context, authType entities.UsageIdentityAuthType) (service.UsageIdentityCostsResponse, error) {
	s.authType = authType
	s.calls++
	return s.response, s.err
}

func TestUsageIdentityCostsReturnsItemsForAIProvider(t *testing.T) {
	provider := &usageIdentityCostProviderStub{response: service.UsageIdentityCostsResponse{
		Items: []service.UsageIdentityCostItem{{
			IdentityID:    "12",
			TotalCostUSD:  12.345678,
			CostAvailable: true,
		}},
		GeneratedAt: time.Date(2026, 8, 20, 10, 0, 0, 0, time.UTC),
	}}
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentityCosts: provider})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/costs?auth_type=2", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	body := resp.Body.String()
	if !contains(body, `"identity_id":"12"`) || !contains(body, `"total_cost_usd":12.345678`) || !contains(body, `"cost_available":true`) {
		t.Fatalf("expected cost items in response body: %s", body)
	}
	if !contains(body, `"generated_at":"2026-08-20T10:00:00Z"`) {
		t.Fatalf("expected generated_at in response body: %s", body)
	}
	if provider.calls != 1 || provider.authType != entities.UsageIdentityAuthTypeAIProvider {
		t.Fatalf("expected provider call with auth_type 2, got calls=%d authType=%d", provider.calls, provider.authType)
	}
}

func TestUsageIdentityCostsAcceptsAuthFileType(t *testing.T) {
	provider := &usageIdentityCostProviderStub{response: service.UsageIdentityCostsResponse{Items: []service.UsageIdentityCostItem{}}}
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentityCosts: provider})
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/costs?auth_type=1", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d body=%s", resp.Code, resp.Body.String())
	}
	if provider.calls != 1 || provider.authType != entities.UsageIdentityAuthTypeAuthFile {
		t.Fatalf("expected provider call with auth_type 1, got calls=%d authType=%d", provider.calls, provider.authType)
	}
}

func TestUsageIdentityCostsRejectsInvalidAuthType(t *testing.T) {
	for _, rawAuthType := range []string{"0", "3", "abc"} {
		t.Run(rawAuthType, func(t *testing.T) {
			provider := &usageIdentityCostProviderStub{}
			router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "", OptionalProviders{UsageIdentityCosts: provider})
			req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/costs?auth_type="+rawAuthType, nil)
			resp := httptest.NewRecorder()

			router.ServeHTTP(resp, req)

			if resp.Code != http.StatusBadRequest {
				t.Fatalf("expected status 400, got %d body=%s", resp.Code, resp.Body.String())
			}
			if provider.calls != 0 {
				t.Fatalf("expected invalid auth_type not to reach provider, got %d calls", provider.calls)
			}
		})
	}
}

func TestUsageIdentityCostsReturnsServerErrorWhenProviderMissing(t *testing.T) {
	router := NewRouter(nil, nil, nil, nil, AuthConfig{}, nil, "")
	req := httptest.NewRequest(http.MethodGet, "/api/v1/usage/identities/costs?auth_type=2", nil)
	resp := httptest.NewRecorder()

	router.ServeHTTP(resp, req)

	if resp.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d body=%s", resp.Code, resp.Body.String())
	}
}
