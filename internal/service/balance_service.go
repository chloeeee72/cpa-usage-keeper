package service

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"cpa-usage-keeper/internal/balance"
	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/repository"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

const (
	tokenRhythmBalanceHost     = "tokenrhythm.studio"
	maxBalanceSessionLength    = 4096
	balanceQueryMaxConcurrency = 4
	balanceQueryOverallTimeout = 10 * time.Second
)

var (
	ErrBalanceUnsupportedType = errors.New("balance query only supports tokenrhythm ai providers")
	ErrBalanceSessionInvalid  = errors.New("balance session is invalid")
)

type BalanceQueryItem struct {
	IdentityID  string                `json:"identity_id"`
	DisplayName string                `json:"display_name"`
	Type        string                `json:"type"`
	Provider    string                `json:"provider"`
	Error       string                `json:"error,omitempty"`
	Summary     *balance.UsageSummary `json:"summary,omitempty"`
}

type BalanceTotals struct {
	BalanceCny          float64 `json:"balance_cny"`
	AvailableBalanceCny float64 `json:"available_balance_cny"`
	FrozenBalanceCny    float64 `json:"frozen_balance_cny"`
	ExpiringBalanceCny  float64 `json:"expiring_balance_cny"`
	Calls               int64   `json:"calls"`
	CostCny             float64 `json:"cost_cny"`
}

type BalanceQueryResponse struct {
	Items           []BalanceQueryItem `json:"items"`
	Totals          BalanceTotals      `json:"totals"`
	ConfiguredCount int                `json:"configured_count"`
	SucceededCount  int                `json:"succeeded_count"`
	FailedCount     int                `json:"failed_count"`
	GeneratedAt     time.Time          `json:"generated_at"`
}

type BalanceProvider interface {
	QueryBalances(context.Context, *int64) (BalanceQueryResponse, error)
	UpdateUsageIdentityBalanceSession(context.Context, int64, string) (entities.UsageIdentity, error)
}

type BalanceService struct {
	db     *gorm.DB
	client *balance.Client
	now    func() time.Time
}

func NewBalanceService(db *gorm.DB, client *balance.Client) BalanceProvider {
	if client == nil {
		client = balance.NewClient(balance.ClientOptions{})
	}
	return &BalanceService{db: db, client: client, now: time.Now}
}

func (s *BalanceService) QueryBalances(ctx context.Context, identityID *int64) (BalanceQueryResponse, error) {
	if identityID != nil {
		return s.querySingle(ctx, *identityID)
	}
	return s.queryAll(ctx)
}

func (s *BalanceService) querySingle(ctx context.Context, identityID int64) (BalanceQueryResponse, error) {
	if identityID <= 0 {
		return BalanceQueryResponse{}, ErrInvalidID
	}
	if s.db == nil {
		return BalanceQueryResponse{}, fmt.Errorf("database is nil")
	}
	identity, err := repository.FindUsageIdentityByID(ctx, s.db.Clauses(dbresolver.Read), identityID)
	if err != nil {
		return BalanceQueryResponse{}, err
	}
	if err := validateBalanceQueryIdentity(identity); err != nil {
		return BalanceQueryResponse{}, err
	}
	item := s.queryIdentity(ctx, identity)
	return buildBalanceQueryResponse([]BalanceQueryItem{item}), nil
}

func (s *BalanceService) queryAll(ctx context.Context) (BalanceQueryResponse, error) {
	if s.db == nil {
		return BalanceQueryResponse{}, fmt.Errorf("database is nil")
	}
	identities, err := repository.ListActiveUsageIdentities(ctx, s.db.Clauses(dbresolver.Read))
	if err != nil {
		return BalanceQueryResponse{}, err
	}
	configured := make([]entities.UsageIdentity, 0, len(identities))
	for _, identity := range identities {
		if identity.AuthType != entities.UsageIdentityAuthTypeAIProvider {
			continue
		}
		if !SupportsTokenRhythmBalance(identity) {
			continue
		}
		if identity.BalanceSession == nil || strings.TrimSpace(*identity.BalanceSession) == "" {
			continue
		}
		configured = append(configured, identity)
	}

	queryCtx, cancel := context.WithTimeout(ctx, balanceQueryOverallTimeout)
	defer cancel()

	items := make([]BalanceQueryItem, len(configured))
	var wg sync.WaitGroup
	sem := make(chan struct{}, balanceQueryMaxConcurrency)
	for index, identity := range configured {
		wg.Add(1)
		go func(index int, identity entities.UsageIdentity) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			items[index] = s.queryIdentity(queryCtx, identity)
		}(index, identity)
	}
	wg.Wait()

	return buildBalanceQueryResponse(items), nil
}

func (s *BalanceService) queryIdentity(ctx context.Context, identity entities.UsageIdentity) BalanceQueryItem {
	item := BalanceQueryItem{
		IdentityID:  formatUsageIdentityID(identity.ID),
		DisplayName: usageIdentityDisplayName(identity),
		Type:        identity.Type,
		Provider:    identity.Provider,
	}
	session := strings.TrimSpace(*identity.BalanceSession)
	summary, err := s.client.QueryUsageSummary(ctx, session)
	if err != nil {
		item.Error = balanceErrorMessage(err)
		return item
	}
	item.Summary = &summary
	return item
}

func (s *BalanceService) UpdateUsageIdentityBalanceSession(ctx context.Context, id int64, session string) (entities.UsageIdentity, error) {
	if id <= 0 {
		return entities.UsageIdentity{}, ErrInvalidID
	}
	if s.db == nil {
		return entities.UsageIdentity{}, fmt.Errorf("database is nil")
	}
	trimmed := strings.TrimSpace(session)
	if trimmed != "" {
		if err := validateBalanceSession(trimmed); err != nil {
			return entities.UsageIdentity{}, err
		}
	}
	identity, err := repository.FindUsageIdentityByID(ctx, s.db.Clauses(dbresolver.Read), id)
	if err != nil {
		return entities.UsageIdentity{}, err
	}
	if identity.IsDeleted {
		return entities.UsageIdentity{}, gorm.ErrRecordNotFound
	}
	if identity.AuthType != entities.UsageIdentityAuthTypeAIProvider || !SupportsTokenRhythmBalance(identity) {
		return entities.UsageIdentity{}, ErrBalanceUnsupportedType
	}

	var value *string
	if trimmed != "" {
		value = &trimmed
	}
	if err := repository.UpdateUsageIdentityBalanceSession(ctx, s.db, id, value); err != nil {
		return entities.UsageIdentity{}, err
	}
	return repository.FindUsageIdentityByID(ctx, s.db.Clauses(dbresolver.Write), id)
}

func SupportsTokenRhythmBalance(item entities.UsageIdentity) bool {
	host := strings.ToLower(tokenRhythmBaseHost(item.BaseURL))
	return host == tokenRhythmBalanceHost || strings.HasSuffix(host, "."+tokenRhythmBalanceHost)
}

func tokenRhythmBaseHost(rawBaseURL string) string {
	rawBaseURL = strings.TrimSpace(rawBaseURL)
	if rawBaseURL == "" {
		return ""
	}
	if !strings.Contains(rawBaseURL, "://") {
		rawBaseURL = "https://" + rawBaseURL
	}
	parsed, err := url.Parse(rawBaseURL)
	if err != nil {
		return ""
	}
	return parsed.Hostname()
}

func validateBalanceQueryIdentity(identity entities.UsageIdentity) error {
	if identity.IsDeleted {
		return gorm.ErrRecordNotFound
	}
	if identity.AuthType != entities.UsageIdentityAuthTypeAIProvider {
		return ErrBalanceUnsupportedType
	}
	if !SupportsTokenRhythmBalance(identity) {
		return ErrBalanceUnsupportedType
	}
	if identity.BalanceSession == nil || strings.TrimSpace(*identity.BalanceSession) == "" {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func validateBalanceSession(session string) error {
	if utf8.RuneCountInString(session) > maxBalanceSessionLength {
		return ErrBalanceSessionInvalid
	}
	for _, r := range session {
		if unicode.IsControl(r) || isDisallowedBalanceSessionFormatRune(r) {
			return ErrBalanceSessionInvalid
		}
	}
	return nil
}

func isDisallowedBalanceSessionFormatRune(r rune) bool {
	switch {
	case r == '\u061c' || r == '\u180e' || r == '\u200b' || r == '\u200c' || r == '\u2060' || r == '\ufeff':
		return true
	case r == '\u200e' || r == '\u200f':
		return true
	case r >= '\u202a' && r <= '\u202e':
		return true
	case r >= '\u2066' && r <= '\u2069':
		return true
	default:
		return false
	}
}

func formatUsageIdentityID(id int64) string {
	return fmt.Sprintf("%d", id)
}

func usageIdentityDisplayName(identity entities.UsageIdentity) string {
	if alias := strings.TrimSpace(stringsOrEmpty(identity.Alias)); alias != "" {
		return alias
	}
	if name := strings.TrimSpace(identity.Name); name != "" {
		return name
	}
	return strings.TrimSpace(identity.Identity)
}

func stringsOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func balanceErrorMessage(err error) string {
	if errors.Is(err, balance.ErrUnauthorized) {
		return "tr_session 未认证（Cookie 失效或格式错误）"
	}
	return err.Error()
}

func buildBalanceQueryResponse(items []BalanceQueryItem) BalanceQueryResponse {
	response := BalanceQueryResponse{
		Items:           items,
		ConfiguredCount: len(items),
		GeneratedAt:     time.Now(),
	}
	for _, item := range items {
		if item.Error != "" {
			response.FailedCount++
			continue
		}
		if item.Summary == nil {
			response.FailedCount++
			continue
		}
		response.SucceededCount++
		response.Totals.BalanceCny += float64(item.Summary.BalanceCny)
		response.Totals.AvailableBalanceCny += float64(item.Summary.AvailableBalanceCny)
		response.Totals.FrozenBalanceCny += float64(item.Summary.FrozenBalanceCny)
		response.Totals.ExpiringBalanceCny += float64(item.Summary.ExpiringBalanceCny)
		response.Totals.Calls += item.Summary.Calls
		response.Totals.CostCny += float64(item.Summary.CostCny)
	}
	return response
}
