package config

import (
	"testing"
	"time"
)

func TestTokenConfiguration(t *testing.T) {
	t.Setenv("ACCESS_TOKEN_EXPIRY", "")
	t.Setenv("REFRESH_TOKEN_EXPIRY", "")
	cfg, err := Load()
	if err != nil || cfg.AccessExpiry != time.Hour || cfg.RefreshExpiry != 30*24*time.Hour {
		t.Fatalf("unexpected default token lifetimes: %+v, %v", cfg, err)
	}
	t.Setenv("ACCESS_TOKEN_EXPIRY", "60")
	t.Setenv("REFRESH_TOKEN_EXPIRY", "120")
	t.Setenv("JWT_SECRET_KEY", "configured-secret")
	cfg, err = Load()
	if err != nil || cfg.AccessExpiry != time.Minute || cfg.RefreshExpiry != 2*time.Minute || cfg.JWTSecret != "configured-secret" {
		t.Fatalf("token configuration not applied: %+v, %v", cfg, err)
	}
	for _, value := range []string{"0", "-1", "invalid", "2147483648"} {
		t.Setenv("ACCESS_TOKEN_EXPIRY", value)
		if _, err := Load(); err == nil {
			t.Fatalf("expected invalid lifetime %q to fail", value)
		}
	}
}
