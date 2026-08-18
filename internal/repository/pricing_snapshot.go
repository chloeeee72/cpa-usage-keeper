package repository

import (
	"context"
	"errors"
	"fmt"

	"cpa-usage-keeper/internal/pricing"

	"gorm.io/gorm"
)

var ErrInvalidPricingSnapshot = errors.New("invalid pricing snapshot")

// LoadPricingSnapshot 从传入的 DB/transaction 一次加载并编译完整价格快照。
func LoadPricingSnapshot(ctx context.Context, db *gorm.DB) (*pricing.Snapshot, error) {
	if db == nil {
		return nil, fmt.Errorf("database is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := db.WithContext(ctx)
	settings, err := ListModelPriceSettings(query)
	if err != nil {
		return nil, err
	}
	rules, err := ListModelPriceRules(query)
	if err != nil {
		return nil, err
	}

	peakHours, err := loadPeakHoursConfig(ctx, query)
	if err != nil {
		return nil, err
	}

	configIndexByID := make(map[int64]int, len(settings))
	configs := make([]pricing.ModelConfig, len(settings))
	for index := range settings {
		configs[index].Pricing = settings[index]
		configIndexByID[settings[index].ID] = index
	}
	for index := range rules {
		configIndex, ok := configIndexByID[rules[index].ModelPriceSettingID]
		if !ok {
			return nil, fmt.Errorf("model price rule %d references missing price %d", rules[index].ID, rules[index].ModelPriceSettingID)
		}
		configs[configIndex].Rules = append(configs[configIndex].Rules, pricing.RuleConfig{
			Key:        rules[index].Key,
			Value:      rules[index].Value,
			Multiplier: rules[index].Multiplier,
		})
	}
	snapshot, err := pricing.CompileSnapshotWithPeakHours(configs, peakHours)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPricingSnapshot, err)
	}
	return snapshot, nil
}

// PeakHoursSettingKey 是 app_settings 中保存高峰时段配置的 key。
const PeakHoursSettingKey = "pricing.peak_hours"

// LoadPeakHoursConfig 读取并解析 app_settings 中的高峰时段配置，供聚合 runner 等非 pricing 路径使用。
func LoadPeakHoursConfig(ctx context.Context, db *gorm.DB) (*pricing.PeakHoursConfig, error) {
	return loadPeakHoursConfig(ctx, db)
}

func loadPeakHoursConfig(ctx context.Context, db *gorm.DB) (*pricing.PeakHoursConfig, error) {
	setting, found, err := GetAppSetting(ctx, db, PeakHoursSettingKey)
	if err != nil {
		return nil, err
	}
	if !found || setting.Value == nil {
		return nil, nil
	}
	config, err := pricing.ParsePeakHoursConfig([]byte(*setting.Value))
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidPricingSnapshot, err)
	}
	return config, nil
}
