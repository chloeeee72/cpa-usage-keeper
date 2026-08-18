package service

import (
	"context"
	"encoding/json"
	"fmt"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/pricing"
	"cpa-usage-keeper/internal/repository"

	"gorm.io/gorm"
)

// GetPeakHours 返回当前生效的高峰时段配置；未配置时返回 DeepSeek 默认值。
func (s *pricingService) GetPeakHours(_ context.Context) (*pricing.PeakHoursConfig, error) {
	if s == nil || s.catalog == nil {
		return nil, fmt.Errorf("pricing service is nil")
	}
	if config := s.catalog.Snapshot().PeakHours(); config != nil {
		return config, nil
	}
	defaultConfig := pricing.DefaultPeakHoursConfig()
	normalized, err := defaultConfig.Normalize()
	if err != nil {
		return nil, fmt.Errorf("normalize default peak hours: %w", err)
	}
	return normalized, nil
}

// UpdatePeakHours 校验并持久化高峰时段配置，然后原子发布新价格快照。
func (s *pricingService) UpdatePeakHours(ctx context.Context, config *pricing.PeakHoursConfig) (*pricing.PeakHoursConfig, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if config == nil {
		return nil, fmt.Errorf("peak hours config is required")
	}
	normalized, err := config.Normalize()
	if err != nil {
		return nil, err
	}
	payload, err := json.Marshal(normalized)
	if err != nil {
		return nil, fmt.Errorf("marshal peak hours config: %w", err)
	}
	value := string(payload)

	_, err = s.mutatePricing(ctx, func(tx *gorm.DB) error {
		_, upsertErr := repository.UpsertAppSetting(ctx, tx, entities.AppSetting{
			SettingKey: repository.PeakHoursSettingKey,
			Value:      &value,
			ValueType:  entities.AppSettingValueTypeJSON,
		})
		return upsertErr
	})
	if err != nil {
		return nil, err
	}
	return normalized, nil
}
