package config

import (
	"testing"
	"time"
)

func TestReadinessConfiguration(t *testing.T) {
	t.Setenv("RABBITMQ_URL", "amqp://localhost/")
	t.Setenv("READINESS_TIMEOUT_SECONDS", "")
	t.Setenv("RECONNECT_INITIAL_DELAY_SECONDS", "")
	t.Setenv("RECONNECT_MAX_DELAY_SECONDS", "")
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ReadinessTimeout != 120*time.Second || cfg.ReconnectInitial != time.Second || cfg.ReconnectMax != 30*time.Second {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	t.Setenv("RECONNECT_INITIAL_DELAY_SECONDS", "31")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid backoff bounds to fail")
	}
	t.Setenv("RECONNECT_INITIAL_DELAY_SECONDS", "1")
	t.Setenv("READINESS_TIMEOUT_SECONDS", "0")
	if _, err := Load(); err == nil {
		t.Fatal("expected invalid readiness timeout to fail")
	}
}
