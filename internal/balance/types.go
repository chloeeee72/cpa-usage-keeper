package balance

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// CnyFloat 兼容 tokenrhythm 返回的金额字段：可能是数字，也可能是字符串。
type CnyFloat float64

func (f *CnyFloat) UnmarshalJSON(data []byte) error {
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "null" {
		*f = 0
		return nil
	}
	if strings.HasPrefix(trimmed, `"`) {
		var text string
		if err := json.Unmarshal(data, &text); err != nil {
			return err
		}
		value, err := strconv.ParseFloat(strings.TrimSpace(text), 64)
		if err != nil {
			return fmt.Errorf("parse cny float %q: %w", text, err)
		}
		*f = CnyFloat(value)
		return nil
	}
	var value float64
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("parse cny float %q: %w", trimmed, err)
	}
	*f = CnyFloat(value)
	return nil
}

// UsageSummary 是 tokenrhythm.studio /api/usage-summary 返回的 data 字段。
type UsageSummary struct {
	BalanceCny          CnyFloat `json:"balanceCny"`
	AvailableBalanceCny CnyFloat `json:"availableBalanceCny"`
	FrozenBalanceCny    CnyFloat `json:"frozenBalanceCny"`
	ExpiringBalanceCny  CnyFloat `json:"expiringBalanceCny"`
	NextExpiryAt        string   `json:"nextExpiryAt"`
	Calls               int64    `json:"calls"`
	CostCny             CnyFloat `json:"costCny"`
}

// HTTPError 保留上游接口返回的 HTTP 状态码与可读错误信息。
type HTTPError struct {
	StatusCode int
	Message    string
}

func (e HTTPError) Error() string {
	if e.Message != "" {
		return fmt.Sprintf("tokenrhythm balance query failed with status %d: %s", e.StatusCode, e.Message)
	}
	return fmt.Sprintf("tokenrhythm balance query failed with status %d", e.StatusCode)
}
