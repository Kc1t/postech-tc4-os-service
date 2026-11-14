package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// CORS responde só para as origens da allowlist (CORS_ALLOWED_ORIGINS). Origem fora da lista
// não recebe Access-Control-Allow-Origin e o navegador bloqueia a resposta; chamadas sem
// Origin (servidor a servidor, curl) seguem normalmente. Um "*" na lista libera qualquer
// origem (a resposta devolve a própria origem, com Vary: Origin).
func CORS(allowed []string) gin.HandlerFunc {
	set := make(map[string]bool, len(allowed))
	allowAll := false
	for _, o := range allowed {
		o = strings.TrimRight(strings.TrimSpace(o), "/")
		if o == "*" {
			allowAll = true
		}
		set[o] = true
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if origin != "" && (allowAll || set[strings.TrimRight(origin, "/")]) {
			h := c.Writer.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Add("Vary", "Origin")
			h.Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
			h.Set("Access-Control-Allow-Headers", "Origin, Content-Type, Authorization, X-Correlation-ID, X-Simulate-Failure")
			h.Set("Access-Control-Expose-Headers", CorrelationHeader)
			h.Set("Access-Control-Max-Age", "600")
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}
