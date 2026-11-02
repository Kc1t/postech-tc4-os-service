package httputil

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// UUIDParams responde 400 quando um dos parâmetros de rota informados existe e não é um UUID,
// antes de chegar ao caso de uso e ao banco. Rotas do grupo sem o parâmetro passam direto.
func UUIDParams(names ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		for _, name := range names {
			v := c.Param(name)
			if v == "" {
				continue
			}
			if err := uuid.Validate(v); err != nil {
				Abort(c, http.StatusBadRequest, CodeValidation, fmt.Sprintf("path parameter '%s' must be a valid UUID", name))
				return
			}
		}
		c.Next()
	}
}
