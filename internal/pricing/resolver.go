package pricing

import (
	"strings"
	"time"

	"cpa-usage-keeper/internal/helper"
)

// CostSubject 是所有 usage 来源进入计价领域的唯一固定输入。
type CostSubject struct {
	Timestamp  time.Time
	Dimensions UsageDimensions
	Tokens     helper.UsageTokenCostInput
}

func NewCostSubject(dimensions UsageDimensions, tokens helper.UsageTokenCostInput) CostSubject {
	return NewCostSubjectWithTimestamp(dimensions, tokens, time.Time{})
}

// NewCostSubjectWithTimestamp 额外携带请求时间，供 Resolver 推导高峰/闲时时段。
func NewCostSubjectWithTimestamp(dimensions UsageDimensions, tokens helper.UsageTokenCostInput, timestamp time.Time) CostSubject {
	return CostSubject{
		Timestamp:  timestamp,
		Dimensions: canonicalizeUsageDimensions(dimensions),
		Tokens:     tokens,
	}
}

type CostResult struct {
	Cost           helper.UsageTokenCostBreakdown
	Available      bool
	PricingStyle   string
	MatchedModel   string
	MatchedBy      string
	RuleMultiplier float64
}

// Resolver 在创建时固定绑定一个 Snapshot，确保单个响应不会混用新旧价格。
type Resolver struct {
	snapshot *Snapshot
}

func (r Resolver) ActiveFields() ActiveFields {
	if r.snapshot == nil {
		return 0
	}
	return r.snapshot.activeFields
}

// PeakHours 返回 Resolver 绑定的高峰时段配置，供聚合查询按同一规则切分。
func (r Resolver) PeakHours() *PeakHoursConfig {
	if r.snapshot == nil {
		return nil
	}
	return r.snapshot.peakHours
}

func (r Resolver) Calculate(subject CostSubject) CostResult {
	dimensions := subject.Dimensions

	model, matchedModel, matchedBy, found := r.matchModel(dimensions)
	if !found {
		return CostResult{
			Available:      !helper.UsageTokenInputRequiresPricing(subject.Tokens),
			RuleMultiplier: 1,
		}
	}

	dimensions.PricingPeriod = r.resolvePricingPeriod(subject, model.peakHours)

	breakdown := helper.CalculateUsageTokenCostBreakdown(subject.Tokens, model.pricing)
	ruleMultiplier := 1.0
	if model.pricing.PriceMultiplier == nil || *model.pricing.PriceMultiplier != 0 {
		ruleMultiplier = matchingRuleMultiplier(model.rules, dimensions)
		breakdown = helper.ScaleUsageTokenCostBreakdown(breakdown, ruleMultiplier)
	}
	return CostResult{
		Cost:           breakdown,
		Available:      true,
		PricingStyle:   model.pricing.PricingStyle,
		MatchedModel:   matchedModel,
		MatchedBy:      matchedBy,
		RuleMultiplier: ruleMultiplier,
	}
}

func (r Resolver) resolvePricingPeriod(subject CostSubject, modelPeakHours *PeakHoursConfig) string {
	if period := strings.TrimSpace(subject.Dimensions.PricingPeriod); period != "" {
		return period
	}
	if subject.Timestamp.IsZero() {
		return string(PricingPeriodPeak)
	}
	peakHours := modelPeakHours
	if peakHours == nil && r.snapshot != nil {
		peakHours = r.snapshot.peakHours
	}
	if peakHours == nil {
		return string(PricingPeriodPeak)
	}
	if peakHours.IsPeak(subject.Timestamp) {
		return string(PricingPeriodPeak)
	}
	return string(PricingPeriodOffPeak)
}

func (r Resolver) matchModel(dimensions UsageDimensions) (compiledModel, string, string, bool) {
	if r.snapshot == nil {
		return compiledModel{}, "", "", false
	}
	if model, ok := r.snapshot.modelsByName[dimensions.Model]; ok {
		return model, dimensions.Model, "model", true
	}
	if model, ok := r.snapshot.modelsByName[dimensions.ModelAlias]; ok {
		return model, dimensions.ModelAlias, "model_alias", true
	}
	return compiledModel{}, "", "", false
}

func matchingRuleMultiplier(rules []compiledRule, dimensions UsageDimensions) float64 {
	multiplier := 1.0
	for _, rule := range rules {
		if dimensions.Value(rule.field) != rule.value {
			continue
		}
		if rule.multiplier == 0 {
			return 0
		}
		multiplier *= rule.multiplier
	}
	return multiplier
}
