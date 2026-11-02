package routes

import (
	"context"
	"net/http"
	"time"

	_ "github.com/Kc1t/postech-tc4-os-service/docs"

	"github.com/Kc1t/postech-tc4-os-service/cmd/api/bootstrap"
	"github.com/Kc1t/postech-tc4-os-service/cmd/api/middleware"
	"github.com/Kc1t/postech-tc4-os-service/internal/domain/entities"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// HealthPaths ficam fora do log de requisições.
var HealthPaths = []string{"/health", "/health/live", "/health/ready"}

// Pinger é o que o readiness precisa do banco.
type Pinger interface {
	PingContext(ctx context.Context) error
}

func Setup(router *gin.Engine, c *bootstrap.Container) {
	router.Use(middleware.CORS(c.Config.CORSAllowedOrigins))
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	sqlDB, _ := c.DB().DB()
	Health(router, sqlDB)

	// Rotas internas (Lambda de autenticação), fora do API Gateway público.
	c.InternalHandler.SetupRoutes(router)

	prefix := router.Group("/api/v1")

	// Rotas publicas
	auth := prefix.Group("/auth")
	auth.POST("/login", c.AuthHandler.Login)
	auth.POST("/refresh", c.AuthHandler.Refresh)

	// Rotas autenticadas (qualquer role)
	protected := prefix.Group("/")
	protected.Use(middleware.Auth(c.Config.JWTSecret))
	protected.POST("/auth/logout", c.AuthHandler.Logout)

	// Rotas do cliente: exigem o JWT emitido a partir do CPF pela lambda de
	// autenticacao, e so respondem para o documento dono do token.
	c.ServiceOrderHandler.SetupRequesterRoutes(protected)

	// Rotas protegidas (JWT obrigatorio) e ADMIN
	admin := prefix.Group("/")
	admin.Use(middleware.Auth(c.Config.JWTSecret))
	admin.Use(middleware.RequireRole(string(entities.RoleAdmin)))

	// Só a equipe cadastra outro usuário da equipe: a rota passa pelo gateway sem authorizer
	// (POST /api/v1/auth/{proxy+}), então a barreira é o JWT de admin validado aqui.
	admin.POST("/auth/register", c.AuthHandler.Register)

	c.RequesterHandler.SetupRoutes(admin)
	c.VehicleHandler.SetupRoutes(admin)
	c.ServiceHandler.SetupRoutes(admin)
	c.PartHandler.SetupRoutes(admin)
	c.ServiceOrderHandler.SetupRoutes(admin)
}

// Health registra as probes. live diz só que o processo responde (o kubelet reinicia o pod se
// falhar); ready verifica o banco (o pod sai do Service enquanto falhar, sem reiniciar).
func Health(router gin.IRouter, db Pinger) {
	live := func(ctx *gin.Context) { ctx.JSON(http.StatusOK, gin.H{"status": "ok"}) }
	router.GET("/health", live)
	router.GET("/health/live", live)
	router.GET("/health/ready", func(ctx *gin.Context) {
		if db == nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "postgres": "not configured"})
			return
		}
		pingCtx, cancel := context.WithTimeout(ctx.Request.Context(), 2*time.Second)
		defer cancel()
		if err := db.PingContext(pingCtx); err != nil {
			ctx.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable", "postgres": "down"})
			return
		}
		ctx.JSON(http.StatusOK, gin.H{"status": "ok", "postgres": "up"})
	})
}
