package pricing

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// PricingPeriod 表示请求发生时所处的高峰/闲时计价时段。
type PricingPeriod string

const (
	PricingPeriodPeak    PricingPeriod = "peak"
	PricingPeriodOffPeak PricingPeriod = "off_peak"
)

// ParsePricingPeriod 把用户输入规范化为固定枚举，避免任意字符串进入规则匹配。
func ParsePricingPeriod(value string) (PricingPeriod, error) {
	switch PricingPeriod(strings.ToLower(strings.TrimSpace(value))) {
	case PricingPeriodPeak:
		return PricingPeriodPeak, nil
	case PricingPeriodOffPeak:
		return PricingPeriodOffPeak, nil
	default:
		return "", fmt.Errorf("invalid pricing period %q", value)
	}
}

// PeakTimeRange 是高峰时段的一个半开区间 [start, end)，格式 HH:MM。
type PeakTimeRange struct {
	Start string `json:"start"`
	End   string `json:"end"`

	startMinutes int
	endMinutes   int
}

// PeakHoursConfig 是全局高峰时段配置，使用 IANA 时区判断请求时间。
type PeakHoursConfig struct {
	Timezone string          `json:"timezone"`
	Ranges   []PeakTimeRange `json:"ranges"`

	location *time.Location
}

// DefaultPeakHoursConfig 返回 DeepSeek 官方默认高峰时段。
func DefaultPeakHoursConfig() *PeakHoursConfig {
	return &PeakHoursConfig{
		Timezone: "Asia/Shanghai",
		Ranges: []PeakTimeRange{
			{Start: "09:00", End: "12:00"},
			{Start: "14:00", End: "18:00"},
		},
	}
}

// ParsePeakHoursConfig 解析并校验 JSON 配置。
func ParsePeakHoursConfig(data []byte) (*PeakHoursConfig, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var config PeakHoursConfig
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("parse peak hours config: %w", err)
	}
	normalized, err := config.Normalize()
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

// Normalize 校验并归一化配置：加载时区、解析时间、排序并合并重叠区间。
func (c *PeakHoursConfig) Normalize() (*PeakHoursConfig, error) {
	if c == nil {
		return nil, nil
	}
	timezone := strings.TrimSpace(c.Timezone)
	if timezone == "" {
		timezone = "Asia/Shanghai"
	}
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return nil, fmt.Errorf("load peak hours timezone %q: %w", timezone, err)
	}

	ranges := make([]PeakTimeRange, 0, len(c.Ranges))
	for _, raw := range c.Ranges {
		start, err := parseClockMinutes(raw.Start)
		if err != nil {
			return nil, fmt.Errorf("peak range start %q: %w", raw.Start, err)
		}
		end, err := parseClockMinutes(raw.End)
		if err != nil {
			return nil, fmt.Errorf("peak range end %q: %w", raw.End, err)
		}
		if start == end {
			return nil, fmt.Errorf("peak range start and end must differ: %q-%q", raw.Start, raw.End)
		}
		ranges = append(ranges, PeakTimeRange{
			Start:        raw.Start,
			End:          raw.End,
			startMinutes: start,
			endMinutes:   end,
		})
	}

	normalized := &PeakHoursConfig{
		Timezone: timezone,
		Ranges:   mergePeakTimeRanges(ranges),
		location: location,
	}
	return normalized, nil
}

// IsPeak 判断某个时间点是否落在高峰时段内。
func (c *PeakHoursConfig) IsPeak(t time.Time) bool {
	if c == nil || c.location == nil || len(c.Ranges) == 0 {
		return false
	}
	local := t.In(c.location)
	minutes := local.Hour()*60 + local.Minute()
	for _, r := range c.Ranges {
		// 普通区间 [start, end)
		if r.startMinutes < r.endMinutes && minutes >= r.startMinutes && minutes < r.endMinutes {
			return true
		}
		// 跨午夜区间 [start, 1440) ∪ [0, end)
		if r.startMinutes >= r.endMinutes && (minutes >= r.startMinutes || minutes < r.endMinutes) {
			return true
		}
	}
	return false
}

// MarshalJSON 只输出持久化字段，内部计算字段不落库。
func (c PeakHoursConfig) MarshalJSON() ([]byte, error) {
	return json.Marshal(struct {
		Timezone string          `json:"timezone"`
		Ranges   []PeakTimeRange `json:"ranges"`
	}{
		Timezone: c.Timezone,
		Ranges:   c.Ranges,
	})
}

func parseClockMinutes(value string) (int, error) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("must be HH:MM")
	}
	var hour, minute int
	if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return 0, fmt.Errorf("invalid hour %q", parts[0])
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minute); err != nil {
		return 0, fmt.Errorf("invalid minute %q", parts[1])
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, fmt.Errorf("time out of range %q", value)
	}
	return hour*60 + minute, nil
}

func mergePeakTimeRanges(ranges []PeakTimeRange) []PeakTimeRange {
	if len(ranges) == 0 {
		return nil
	}
	sorted := append([]PeakTimeRange(nil), ranges...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].startMinutes != sorted[j].startMinutes {
			return sorted[i].startMinutes < sorted[j].startMinutes
		}
		return sorted[i].endMinutes < sorted[j].endMinutes
	})

	merged := make([]PeakTimeRange, 0, len(sorted))
	for _, current := range sorted {
		if len(merged) == 0 {
			merged = append(merged, current)
			continue
		}
		last := &merged[len(merged)-1]
		// 跨午夜区间单独处理：不参与普通合并，保持原样。
		if current.startMinutes >= current.endMinutes || last.startMinutes >= last.endMinutes {
			merged = append(merged, current)
			continue
		}
		if current.startMinutes <= last.endMinutes {
			if current.endMinutes > last.endMinutes {
				last.endMinutes = current.endMinutes
				last.End = current.End
			}
			continue
		}
		merged = append(merged, current)
	}
	return merged
}
