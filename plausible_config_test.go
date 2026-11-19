//go:build integration

package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlausibleConfiguration(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://reader:example@content.example/photos")
	t.Setenv("USERS_DATABASE_URL", "postgres://writer:example@localhost/users")
	path := filepath.Join(t.TempDir(), ".env")
	cfg, err := loadConfigFile(path)
	if err != nil || cfg.PlausibleEnabled || cfg.PlausibleDomain != "" || cfg.PlausibleScriptURL != "" {
		t.Fatalf("tracking must be optional and disabled without settings: %v", err)
	}
	content := "PLAUSIBLE_DOMAIN=' Reloaded.CatsOf.Asia '\nPLAUSIBLE_SCRIPT_URL=' https://analytics.example.test/js/script.js '\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfigFile(path)
	if err != nil || cfg.PlausibleEnabled {
		t.Fatalf("domain and URL alone must not enable tracking: %v", err)
	}
	if cfg.PlausibleDomain != "reloaded.catsof.asia" || cfg.PlausibleScriptURL != "https://analytics.example.test/js/script.js" {
		t.Fatal("tracking values were not loaded and normalized")
	}
	if err := os.WriteFile(path, []byte(content+"PLAUSIBLE_ENABLED=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfigFile(path)
	if err != nil || !cfg.PlausibleEnabled {
		t.Fatalf("explicit file setting did not enable tracking: %v", err)
	}
	t.Setenv("PLAUSIBLE_ENABLED", "false")
	t.Setenv("PLAUSIBLE_DOMAIN", "other.example.test")
	t.Setenv("PLAUSIBLE_SCRIPT_URL", "https://other.example.test/custom.js")
	cfg, err = loadConfigFile(path)
	if err != nil || cfg.PlausibleEnabled || cfg.PlausibleDomain != "other.example.test" || cfg.PlausibleScriptURL != "https://other.example.test/custom.js" {
		t.Fatalf("environment settings did not override the file: %v", err)
	}
	t.Setenv("PLAUSIBLE_ENABLED", "true")
	t.Setenv("PLAUSIBLE_DOMAIN", "")
	if _, err := loadConfigFile(path); err == nil || !strings.Contains(err.Error(), "PLAUSIBLE_DOMAIN") {
		t.Fatal("an explicitly empty domain did not override the file or fail validation")
	}
	t.Setenv("PLAUSIBLE_ENABLED", "invalid-private-example")
	if _, err := loadConfigFile(path); err == nil || err.Error() != "cannot parse environment configuration" {
		t.Fatal("an invalid tracking switch must fail without exposing its value")
	}
}
