package api

import (
	"net/http"
	"strconv"
	"strings"

	"cpa-usage-keeper/internal/entities"
	"cpa-usage-keeper/internal/service"

	"github.com/gin-gonic/gin"
)

func registerUsageIdentityCostRoutes(router gin.IRoutes, provider service.UsageIdentityCostProvider) {
	router.GET("/usage/identities/costs", func(c *gin.Context) {
		if provider == nil {
			writeInternalError(c, "usage identity cost provider is not configured", nil)
			return
		}
		authType := entities.UsageIdentityAuthTypeAIProvider
		if rawAuthType := strings.TrimSpace(c.Query("auth_type")); rawAuthType != "" {
			value, err := strconv.Atoi(rawAuthType)
			if err != nil || (value != int(entities.UsageIdentityAuthTypeAuthFile) && value != int(entities.UsageIdentityAuthTypeAIProvider)) {
				c.JSON(http.StatusBadRequest, gin.H{"error": "auth_type must be 1 or 2"})
				return
			}
			authType = entities.UsageIdentityAuthType(value)
		}
		response, err := provider.ListUsageIdentityCosts(c.Request.Context(), authType)
		if err != nil {
			writeInternalError(c, "list usage identity costs failed", err)
			return
		}
		c.JSON(http.StatusOK, response)
	})
}
