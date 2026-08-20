package balance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const DefaultBaseURL = "https://tokenrhythm.studio"

var ErrUnauthorized = errors.New("tokenrhythm session is unauthorized")

type ClientOptions struct {
	BaseURL    string
	HTTPClient *http.Client
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

func NewClient(options ClientOptions) *Client {
	baseURL := strings.TrimRight(strings.TrimSpace(options.BaseURL), "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}
	httpClient := options.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 8 * time.Second}
	}
	return &Client{baseURL: baseURL, httpClient: httpClient}
}

func (c *Client) QueryUsageSummary(ctx context.Context, session string) (UsageSummary, error) {
	var summary UsageSummary
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/usage-summary", nil)
	if err != nil {
		return summary, fmt.Errorf("build tokenrhythm balance request: %w", err)
	}
	req.Header.Set("Cookie", "tr_session="+session)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return summary, fmt.Errorf("request tokenrhythm balance: %w", err)
	}
	defer resp.Body.Close()

	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if readErr != nil {
		return summary, fmt.Errorf("read tokenrhythm balance response: %w", readErr)
	}

	if resp.StatusCode == http.StatusUnauthorized {
		return summary, ErrUnauthorized
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return summary, HTTPError{StatusCode: resp.StatusCode, Message: errorMessageFromBody(body)}
	}

	var payload struct {
		Data UsageSummary `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return summary, fmt.Errorf("parse tokenrhythm balance response: %w", err)
	}
	return payload.Data, nil
}

func errorMessageFromBody(body []byte) string {
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return ""
	}
	var object map[string]any
	if err := json.Unmarshal(body, &object); err != nil {
		return trimmed
	}
	for _, key := range []string{"error", "message", "detail"} {
		if message := stringField(object, key); message != "" {
			return message
		}
	}
	return trimmed
}

func stringField(object map[string]any, key string) string {
	value, ok := object[key]
	if !ok {
		return ""
	}
	text, ok := value.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(text)
}
