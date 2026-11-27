package daemon

import (
	"testing"
	"time"
)

func TestConfigFromEnv(t *testing.T) {
	t.Setenv("AGENT_PWD_IDLE_TIMEOUT", "12h")
	t.Setenv("AGENT_PWD_HARD_TIMEOUT", "36h")
	t.Setenv("AGENT_PWD_CONFIRM_TTL", "10m")
	t.Setenv("AGENT_PWD_FILL_TTL", "120s")
	cfg := ConfigFromEnv()
	if cfg.IdleTimeout != 12*time.Hour || cfg.HardTimeout != 36*time.Hour ||
		cfg.ConfirmTTL != 10*time.Minute || cfg.FillTTL != 120*time.Second {
		t.Fatalf("env overrides not applied: %+v", cfg)
	}

	t.Setenv("AGENT_PWD_IDLE_TIMEOUT", "garbage")
	t.Setenv("AGENT_PWD_HARD_TIMEOUT", "1h")
	cfg = ConfigFromEnv()
	if cfg.IdleTimeout != DefaultConfig().IdleTimeout {
		t.Fatalf("invalid duration was accepted: %+v", cfg)
	}
	if cfg.HardTimeout < cfg.IdleTimeout {
		t.Fatalf("hard timeout was not clamped: %+v", cfg)
	}
}
