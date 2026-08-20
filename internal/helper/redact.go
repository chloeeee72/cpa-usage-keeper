package helper

import (
	"strings"

	"cpa-usage-keeper/internal/entities"
)

const sensitiveValueMask = "*********"

// RedactSensitiveValue 使用统一格式隐藏前端展示中的敏感值：长值保留前 3 位和后 6 位，短值全隐藏。
func RedactSensitiveValue(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" || trimmed == "unknown" {
		return "unknown"
	}
	runes := []rune(trimmed)
	if len(runes) <= 9 {
		return sensitiveValueMask
	}
	return string(runes[:3]) + sensitiveValueMask + string(runes[len(runes)-6:])
}

// MaskAPIKeyDisplay 按“前 6 位 + ***** + 后 5 位”展示 API Key；长度不足以切分时整体遮蔽。
func MaskAPIKeyDisplay(value string) string {
	trimmed := strings.TrimSpace(value)
	runes := []rune(trimmed)
	if len(runes) <= 11 {
		return sensitiveValueMask
	}
	return string(runes[:6]) + "*****" + string(runes[len(runes)-5:])
}

// CPAAPIKeyMaskedDisplayKey 返回 CPA API Key 的安全展示 key；优先基于原始 key 重新脱敏，避免历史 DisplayKey 格式不一致。
func CPAAPIKeyMaskedDisplayKey(row entities.CPAAPIKey) string {
	if strings.TrimSpace(row.APIKey) != "" {
		return RedactSensitiveValue(row.APIKey)
	}
	return strings.TrimSpace(row.DisplayKey)
}

// CPAAPIKeyDisplayName 返回 CPA API Key 的前端展示名：优先别名，其次使用统一脱敏 key。
func CPAAPIKeyDisplayName(row entities.CPAAPIKey) string {
	if strings.TrimSpace(row.KeyAlias) != "" {
		return strings.TrimSpace(row.KeyAlias)
	}
	return CPAAPIKeyMaskedDisplayKey(row)
}
