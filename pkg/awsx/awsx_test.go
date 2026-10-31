package awsx

import (
	"context"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	t.Setenv("AWS_REGION", "us-east-1")
	t.Setenv("AWS_ACCESS_KEY_ID", "test")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test")
	cfg, err := LoadConfig(context.Background())
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if cfg.Region != "us-east-1" {
		t.Fatalf("região = %q", cfg.Region)
	}
	if len(cfg.APIOptions) == 0 {
		t.Fatal("middleware do New Relic não registrado")
	}
}
