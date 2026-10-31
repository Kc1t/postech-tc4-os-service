package config

import (
	"os"
	"testing"
	"time"

	"github.com/Kc1t/postech-tc4-os-service/pkg/env"
)

func TestLoad_Defaults(t *testing.T) {
	t.Helper()
	for _, key := range []string{"APP_PORT", "APP_ENV", "POSTGRES_DSN", "JWT_SECRET"} {
		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("Unsetenv(%q): %v", key, err)
		}
	}

	cfg := Load()

	if cfg.AppPort != "8080" {
		t.Errorf("expected AppPort %q, got %q", "8080", cfg.AppPort)
	}
	if cfg.AppEnv != "development" {
		t.Errorf("expected AppEnv %q, got %q", "development", cfg.AppEnv)
	}
	if cfg.SagaApprovalTimeout != 72*time.Hour || cfg.SagaPaymentTimeout != 24*time.Hour || cfg.FaultInjectionEnabled {
		t.Errorf("padrões da saga: %v %v %v", cfg.SagaApprovalTimeout, cfg.SagaPaymentTimeout, cfg.FaultInjectionEnabled)
	}
	if cfg.MessagingEnabled() {
		t.Error("sem filas configuradas a mensageria fica desligada")
	}
	if cfg.PostgresDSN == "" {
		t.Error("expected non-empty PostgresDSN")
	}
}

func TestLoad_EnvOverride(t *testing.T) {
	envs := map[string]string{
		"APP_PORT":   "9090",
		"APP_ENV":    "production",
		"JWT_SECRET": "my-secret",
	}
	for k, v := range envs {
		if err := os.Setenv(k, v); err != nil {
			t.Fatalf("Setenv(%q): %v", k, err)
		}
	}
	defer func() {
		for k := range envs {
			_ = os.Unsetenv(k)
		}
	}()

	cfg := Load()

	if cfg.AppPort != "9090" {
		t.Errorf("expected AppPort %q, got %q", "9090", cfg.AppPort)
	}
	if cfg.AppEnv != "production" {
		t.Errorf("expected AppEnv %q, got %q", "production", cfg.AppEnv)
	}
	if cfg.JWTSecret != "my-secret" {
		t.Errorf("expected JWTSecret %q, got %q", "my-secret", cfg.JWTSecret)
	}
}

func TestGetEnv_WithValue(t *testing.T) {
	if err := os.Setenv("TEST_KEY_XYZ", "test-value"); err != nil {
		t.Fatalf("Setenv: %v", err)
	}
	defer func() { _ = os.Unsetenv("TEST_KEY_XYZ") }()

	result := env.GetOrDefault("TEST_KEY_XYZ", "fallback")
	if result != "test-value" {
		t.Errorf("expected %q, got %q", "test-value", result)
	}
}

func TestGetEnv_WithFallback(t *testing.T) {
	if err := os.Unsetenv("TEST_KEY_MISSING"); err != nil {
		t.Fatalf("Unsetenv: %v", err)
	}
	result := env.GetOrDefault("TEST_KEY_MISSING", "fallback")
	if result != "fallback" {
		t.Errorf("expected %q, got %q", "fallback", result)
	}
}

func TestMessagingEnabled(t *testing.T) {
	t.Setenv("SQS_SAGA_REPLIES_URL", "http://q/os-saga-replies")
	t.Setenv("SQS_BILLING_COMMANDS_URL", "http://q/billing-commands")
	t.Setenv("SQS_EXECUTION_COMMANDS_URL", "http://q/execution-commands")
	t.Setenv("SNS_OS_EVENTS_ARN", "arn:aws:sns:us-east-1:000000000000:os-events")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://a.com,https://b.com")
	t.Setenv("SAGA_APPROVAL_TIMEOUT", "2m")
	cfg := Load()
	if !cfg.MessagingEnabled() || len(cfg.CORSAllowedOrigins) != 2 || cfg.SagaApprovalTimeout != 2*time.Minute {
		t.Fatalf("config: %+v", cfg)
	}
}
