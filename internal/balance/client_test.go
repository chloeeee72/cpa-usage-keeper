package balance

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestQueryUsageSummaryParsesData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/usage-summary" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if cookie := r.Header.Get("Cookie"); !strings.Contains(cookie, "tr_session=session-a") {
			t.Fatalf("expected tr_session cookie, got %q", cookie)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"balanceCny":12.5,"availableBalanceCny":10,"frozenBalanceCny":2.5,"expiringBalanceCny":0,"nextExpiryAt":"2026-09-01","calls":42,"costCny":3.2}}`))
	}))
	defer server.Close()

	client := NewClient(ClientOptions{BaseURL: server.URL})
	summary, err := client.QueryUsageSummary(context.Background(), "session-a")
	if err != nil {
		t.Fatalf("QueryUsageSummary returned error: %v", err)
	}
	if summary.BalanceCny != 12.5 || summary.Calls != 42 || summary.CostCny != 3.2 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestQueryUsageSummaryParsesStringCnyFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"balanceCny":"9.61206560","availableBalanceCny":"9.61206560","frozenBalanceCny":"0.00000000","expiringBalanceCny":"9.61206560","nextExpiryAt":"2026-09-17T19:43:11.275Z","calls":17,"costCny":"0.38793440"}}`))
	}))
	defer server.Close()

	client := NewClient(ClientOptions{BaseURL: server.URL})
	summary, err := client.QueryUsageSummary(context.Background(), "session-string-cny")
	if err != nil {
		t.Fatalf("QueryUsageSummary returned error: %v", err)
	}
	if float64(summary.BalanceCny) != 9.61206560 || float64(summary.CostCny) != 0.38793440 || summary.Calls != 17 {
		t.Fatalf("unexpected summary: %+v", summary)
	}
}

func TestQueryUsageSummaryUnauthorized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer server.Close()

	client := NewClient(ClientOptions{BaseURL: server.URL})
	_, err := client.QueryUsageSummary(context.Background(), "expired")
	if err != ErrUnauthorized {
		t.Fatalf("expected ErrUnauthorized, got %v", err)
	}
}

func TestQueryUsageSummaryHTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"upstream down"}`))
	}))
	defer server.Close()

	client := NewClient(ClientOptions{BaseURL: server.URL})
	_, err := client.QueryUsageSummary(context.Background(), "session-b")
	var httpErr HTTPError
	if !asHTTPError(err, &httpErr) {
		t.Fatalf("expected HTTPError, got %v", err)
	}
	if httpErr.StatusCode != http.StatusBadGateway || !strings.Contains(httpErr.Message, "upstream down") {
		t.Fatalf("unexpected HTTPError: %+v", httpErr)
	}
}

func asHTTPError(err error, target *HTTPError) bool {
	if err == nil {
		return false
	}
	httpErr, ok := err.(HTTPError)
	if !ok {
		return false
	}
	*target = httpErr
	return true
}
