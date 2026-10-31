package config

import (
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/env"
	"github.com/joho/godotenv"
)

type AppEnv string

const (
	EnvDevelopment AppEnv = "development"
	EnvProduction  AppEnv = "prod"
)

type Config struct {
	AppPort            string
	AppEnv             AppEnv
	LogLevel           string
	NewRelicAppName    string
	NewRelicLicenseKey string
	PostgresDSN        string

	JWTSecret           string
	AccessTokenExpMin   int
	RefreshTokenExpDays int
	BcryptCost          int
	MaxFailedLogins     int
	LoginLockMin        int

	// CORSAllowedOrigins é a allowlist de origens do navegador (nunca "*").
	CORSAllowedOrigins []string
	// InternalAPIToken protege /internal/* (consulta da Lambda de autenticação).
	InternalAPIToken string

	// Mensageria (SNS/SQS). Sem as URLs, o serviço sobe só com a API HTTP.
	SagaRepliesQueueURL       string
	BillingCommandsQueueURL   string
	ExecutionCommandsQueueURL string
	OSEventsTopicARN          string
	OutboxPollInterval        time.Duration

	// Saga.
	SagaApprovalTimeout   time.Duration
	SagaPaymentTimeout    time.Duration
	SagaTimeoutCheck      time.Duration
	FaultInjectionEnabled bool

	// API composition.
	BillingServiceURL   string
	ExecutionServiceURL string
	UpstreamTimeout     time.Duration

	ShutdownTimeout time.Duration
}

// MessagingEnabled indica se os workers de mensageria (relay do outbox, consumidor das
// respostas da saga e verificação de prazos) devem rodar.
func (c *Config) MessagingEnabled() bool {
	return c.SagaRepliesQueueURL != "" && c.BillingCommandsQueueURL != "" &&
		c.ExecutionCommandsQueueURL != "" && c.OSEventsTopicARN != ""
}

func Load() *Config {
	_ = godotenv.Load()

	return &Config{
		AppPort:            env.GetOrDefault("APP_PORT", "8080"),
		AppEnv:             AppEnv(env.GetOrDefault("APP_ENV", string(EnvDevelopment))),
		LogLevel:           env.GetOrDefault("LOG_LEVEL", "info"),
		NewRelicAppName:    env.GetOrDefault("NEW_RELIC_APP_NAME", "os-service"),
		NewRelicLicenseKey: env.GetOrDefault("NEW_RELIC_LICENSE_KEY", ""),
		PostgresDSN:        env.GetOrDefault("POSTGRES_DSN", "postgres://postgres:postgres@localhost:5432/workshop?sslmode=disable"),

		JWTSecret:           env.GetOrDefault("JWT_SECRET", "change-me-in-production"),
		AccessTokenExpMin:   env.GetIntOrDefault("ACCESS_TOKEN_EXP_MIN", 15),
		RefreshTokenExpDays: env.GetIntOrDefault("REFRESH_TOKEN_EXP_DAYS", 7),
		BcryptCost:          env.GetIntOrDefault("BCRYPT_COST", 12),
		MaxFailedLogins:     env.GetIntOrDefault("MAX_FAILED_LOGINS", 5),
		LoginLockMin:        env.GetIntOrDefault("LOGIN_LOCK_MIN", 15),

		CORSAllowedOrigins: env.GetListOrDefault("CORS_ALLOWED_ORIGINS", []string{"http://localhost:3000"}),
		InternalAPIToken:   env.GetOrDefault("INTERNAL_API_TOKEN", ""),

		SagaRepliesQueueURL:       env.GetOrDefault("SQS_SAGA_REPLIES_URL", ""),
		BillingCommandsQueueURL:   env.GetOrDefault("SQS_BILLING_COMMANDS_URL", ""),
		ExecutionCommandsQueueURL: env.GetOrDefault("SQS_EXECUTION_COMMANDS_URL", ""),
		OSEventsTopicARN:          env.GetOrDefault("SNS_OS_EVENTS_ARN", ""),
		OutboxPollInterval:        env.GetDurationOrDefault("OUTBOX_POLL_INTERVAL", 500*time.Millisecond),

		SagaApprovalTimeout:   env.GetDurationOrDefault("SAGA_APPROVAL_TIMEOUT", 72*time.Hour),
		SagaPaymentTimeout:    env.GetDurationOrDefault("SAGA_PAYMENT_TIMEOUT", 24*time.Hour),
		SagaTimeoutCheck:      env.GetDurationOrDefault("SAGA_TIMEOUT_CHECK_INTERVAL", time.Minute),
		FaultInjectionEnabled: env.GetBoolOrDefault("FAULT_INJECTION_ENABLED", false),

		BillingServiceURL:   env.GetOrDefault("BILLING_SERVICE_URL", "http://localhost:8082"),
		ExecutionServiceURL: env.GetOrDefault("EXECUTION_SERVICE_URL", "http://localhost:8083"),
		UpstreamTimeout:     env.GetDurationOrDefault("UPSTREAM_TIMEOUT", 2*time.Second),

		ShutdownTimeout: env.GetDurationOrDefault("SHUTDOWN_TIMEOUT", 20*time.Second),
	}
}
