package service

import (
	"context"
	"fmt"
	"time"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/helper"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"

	"gorm.io/gorm"
	"gorm.io/plugin/dbresolver"
)

type UsageIdentityCostItem struct {
	IdentityID    string  `json:"identity_id"`
	TotalCostUSD  float64 `json:"total_cost_usd"`
	CostAvailable bool    `json:"cost_available"`
}

type UsageIdentityCostsResponse struct {
	Items       []UsageIdentityCostItem `json:"items"`
	GeneratedAt time.Time               `json:"generated_at"`
}

type UsageIdentityCostProvider interface {
	ListUsageIdentityCosts(context.Context, entities.UsageIdentityAuthType) (UsageIdentityCostsResponse, error)
}

type usageIdentityCostService struct {
	db      *gorm.DB
	catalog *pricing.Catalog
	now     func() time.Time
}

func NewUsageIdentityCostService(db *gorm.DB, catalog *pricing.Catalog) UsageIdentityCostProvider {
	return &usageIdentityCostService{db: db, catalog: catalog, now: time.Now}
}

func (s *usageIdentityCostService) ListUsageIdentityCosts(ctx context.Context, authType entities.UsageIdentityAuthType) (UsageIdentityCostsResponse, error) {
	response := UsageIdentityCostsResponse{Items: []UsageIdentityCostItem{}, GeneratedAt: s.now()}
	if s.db == nil {
		return response, fmt.Errorf("database is nil")
	}
	identities, err := repository.ListActiveUsageIdentities(ctx, s.db.Clauses(dbresolver.Read))
	if err != nil {
		return response, err
	}
	idByAuthIndex := make(map[string]string)
	authIndexes := make([]string, 0)
	for _, identity := range identities {
		if identity.AuthType != authType {
			continue
		}
		authIndex := identity.Identity
		authIndexes = append(authIndexes, authIndex)
		idByAuthIndex[authIndex] = formatUsageIdentityID(identity.ID)
	}
	if len(authIndexes) == 0 {
		return response, nil
	}

	var resolver pricing.Resolver
	activeFields := pricing.ActiveFields(0)
	if s.catalog != nil {
		resolver = s.catalog.NewResolver()
		activeFields = resolver.ActiveFields()
	}
	rows, err := repository.AggregateUsageIdentityCosts(ctx, s.db.Clauses(dbresolver.Read), authIndexes, activeFields, response.GeneratedAt)
	if err != nil {
		return response, err
	}

	costByAuthIndex := make(map[string]float64)
	availableByAuthIndex := make(map[string]bool)
	for _, row := range rows {
		var cost float64
		available := false
		if s.catalog != nil {
			result := resolver.Calculate(pricing.NewCostSubject(pricing.UsageDimensions{
				APIGroupKey:         row.APIGroupKey,
				Model:               row.Model,
				AuthIndex:           row.AuthIndex,
				ModelAlias:          row.ModelAlias,
				ServiceTier:         row.ServiceTier,
				ResponseServiceTier: row.ResponseServiceTier,
				ReasoningEffort:     row.ReasoningEffort,
				Endpoint:            row.Endpoint,
				ExecutorType:        row.ExecutorType,
				PricingPeriod:       row.PricingPeriod,
			}, helper.UsageTokenCostInput{
				InputTokens:         row.CostUncachedInputTokens + row.CostCacheReadTokens + row.CostCacheCreationTokens,
				OutputTokens:        row.CostOutputTokens,
				CacheReadTokens:     row.CostCacheReadTokens,
				CacheCreationTokens: row.CostCacheCreationTokens,
			}))
			cost = result.Cost.TotalCostUSD
			available = result.Available
		}
		costByAuthIndex[row.AuthIndex] += cost
		if current, ok := availableByAuthIndex[row.AuthIndex]; !ok {
			availableByAuthIndex[row.AuthIndex] = available
		} else if current && !available {
			availableByAuthIndex[row.AuthIndex] = false
		}
	}

	for authIndex, identityID := range idByAuthIndex {
		available, ok := availableByAuthIndex[authIndex]
		if !ok {
			available = true
		}
		response.Items = append(response.Items, UsageIdentityCostItem{
			IdentityID:    identityID,
			TotalCostUSD:  costByAuthIndex[authIndex],
			CostAvailable: available,
		})
	}
	return response, nil
}
