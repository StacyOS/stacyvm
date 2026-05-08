package main

import (
	"testing"

	"github.com/StacyOs/stacyvm/internal/config"
)

func TestSeverityForProduction(t *testing.T) {
	if got := severityForProduction(false); got != doctorWarn {
		t.Fatalf("non-production severity = %s, want %s", got, doctorWarn)
	}
	if got := severityForProduction(true); got != doctorFail {
		t.Fatalf("production severity = %s, want %s", got, doctorFail)
	}
}

func TestCheckConfigProductionAuthPosture(t *testing.T) {
	cfg := &config.Config{}
	cfg.Auth.APIKey = "short"
	cfg.Auth.AdminAPIKey = "short"
	cfg.Auth.AdminFallbackEnabled = true
	cfg.Database.Path = "stacyvm.db"

	checks := checkConfig(cfg, true)
	statuses := map[string]doctorStatus{}
	for _, check := range checks {
		statuses[check.Name] = check.Status
	}

	for _, name := range []string{"auth.api_key", "auth.admin_api_key", "auth.admin_fallback_enabled"} {
		if statuses[name] != doctorFail {
			t.Fatalf("%s status = %s, want %s", name, statuses[name], doctorFail)
		}
	}
}
