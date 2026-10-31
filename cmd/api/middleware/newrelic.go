package middleware

import (
	"github.com/gin-gonic/gin"
	"github.com/newrelic/go-agent/v3/integrations/nrgin"
	"github.com/newrelic/go-agent/v3/newrelic"
)

// NewRelicContext coloca a transação do nrgin no context.Context da requisição. O nrgin guarda
// a transação só no gin.Context; sem isso, banco, outbox (headers de trace das mensagens) e
// chamadas HTTP feitas com c.Request.Context() não se ligam à transação.
func NewRelicContext() gin.HandlerFunc {
	return func(c *gin.Context) {
		if txn := nrgin.Transaction(c); txn != nil {
			c.Request = c.Request.WithContext(newrelic.NewContext(c.Request.Context(), txn))
		}
		c.Next()
	}
}
