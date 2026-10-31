package middleware

import (
	"net/http"
	"strings"

	httputil "github.com/Kc1t/postech-tc4-os-service/internal/adapters/inbound/http"
	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
)

func Auth(jwtSecret string) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			unauthorized(c, "authorization header required")
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "bearer") {
			unauthorized(c, "invalid authorization header format")
			return
		}

		token, err := jwt.Parse(parts[1], func(t *jwt.Token) (interface{}, error) {
			if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, jwt.ErrSignatureInvalid
			}
			return []byte(jwtSecret), nil
		})
		if err != nil || !token.Valid {
			unauthorized(c, "invalid or expired token")
			return
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			unauthorized(c, "invalid token claims")
			return
		}

		sub, subOk := claims["sub"].(string)
		role, roleOk := claims["role"].(string)
		if !subOk || sub == "" || !roleOk || role == "" {
			unauthorized(c, "incomplete token claims")
			return
		}

		c.Set("claims", claims)
		c.Set("user_id", sub)
		c.Set("user_role", role)
		if email, ok := claims["email"].(string); ok {
			c.Set("user_email", email)
		}
		if document, ok := claims["document"].(string); ok {
			c.Set("user_document", document)
		}

		c.Next()
	}
}

func unauthorized(c *gin.Context, msg string) {
	httputil.Abort(c, http.StatusUnauthorized, httputil.CodeUnauthorized, msg)
}
