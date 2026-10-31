package middleware

import (
	"net/http"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/gin-gonic/gin"
)

func RequireRole(allowed ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("user_role")
		if !exists {
			httputil.Abort(c, http.StatusForbidden, httputil.CodeForbidden, "forbidden: role not found")
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			httputil.Abort(c, http.StatusForbidden, httputil.CodeForbidden, "forbidden: invalid role")
			return
		}

		for _, a := range allowed {
			if roleStr == a {
				c.Next()
				return
			}
		}

		httputil.Abort(c, http.StatusForbidden, httputil.CodeForbidden, "insufficient permissions")
	}
}
