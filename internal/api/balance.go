package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type updateBalanceSessionRequest struct {
	Session *string `json:"session"`
}

type balanceQueryRequest struct {
	IdentityID *string `json:"identity_id"`
}

func registerBalanceRoutes(router gin.IRoutes, provider service.BalanceProvider) {
	router.PATCH("/usage/identities/:id/balance-session", func(c *gin.Context) {
		if provider == nil {
			writeInternalError(c, "balance provider is not configured", nil)
			return
		}
		id, err := strconv.ParseInt(strings.TrimSpace(c.Param("id")), 10, 64)
		if err != nil || id <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage identity id"})
			return
		}
		session, ok := parseBalanceSessionRequest(c)
		if !ok {
			return
		}
		row, err := provider.UpdateUsageIdentityBalanceSession(c.Request.Context(), id, session)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidID):
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid usage identity id"})
			case errors.Is(err, service.ErrBalanceSessionInvalid):
				c.JSON(http.StatusBadRequest, gin.H{"error": "balance session is invalid"})
			case errors.Is(err, service.ErrBalanceUnsupportedType):
				c.JSON(http.StatusBadRequest, gin.H{"error": "balance session is only supported for tokenrhythm ai providers"})
			case errors.Is(err, gorm.ErrRecordNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "usage identity not found"})
			default:
				writeInternalError(c, "update usage identity balance session failed", err)
			}
			return
		}
		c.JSON(http.StatusOK, mapUsageIdentityResponse(row))
	})

	router.POST("/usage/balance/query", func(c *gin.Context) {
		if provider == nil {
			writeInternalError(c, "balance provider is not configured", nil)
			return
		}
		identityID, ok := parseBalanceQueryRequest(c)
		if !ok {
			return
		}
		response, err := provider.QueryBalances(c.Request.Context(), identityID)
		if err != nil {
			switch {
			case errors.Is(err, service.ErrInvalidID):
				c.JSON(http.StatusBadRequest, gin.H{"error": "identity_id must be a valid usage identity id"})
			case errors.Is(err, service.ErrBalanceUnsupportedType):
				c.JSON(http.StatusBadRequest, gin.H{"error": "balance query is only supported for tokenrhythm ai providers"})
			case errors.Is(err, gorm.ErrRecordNotFound):
				c.JSON(http.StatusNotFound, gin.H{"error": "usage identity not found or balance session not configured"})
			default:
				writeInternalError(c, "balance query failed", err)
			}
			return
		}
		c.JSON(http.StatusOK, response)
	})
}

func parseBalanceSessionRequest(c *gin.Context) (string, bool) {
	var payload map[string]json.RawMessage
	if err := c.ShouldBindJSON(&payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return "", false
	}
	rawSession, ok := payload["session"]
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session is required"})
		return "", false
	}
	if bytes.Equal(bytes.TrimSpace(rawSession), []byte("null")) {
		return "", true
	}
	var session string
	if err := json.Unmarshal(rawSession, &session); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "session must be a string or null"})
		return "", false
	}
	return strings.TrimSpace(session), true
}

func parseBalanceQueryRequest(c *gin.Context) (*int64, bool) {
	raw, err := c.GetRawData()
	if err != nil && !errors.Is(err, io.EOF) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return nil, false
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		return nil, true
	}
	var request balanceQueryRequest
	if err := json.Unmarshal(raw, &request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return nil, false
	}
	if request.IdentityID == nil {
		return nil, true
	}
	trimmed := strings.TrimSpace(*request.IdentityID)
	if trimmed == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "identity_id must be a valid usage identity id"})
		return nil, false
	}
	id, err := strconv.ParseInt(trimmed, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "identity_id must be a valid usage identity id"})
		return nil, false
	}
	return &id, true
}
