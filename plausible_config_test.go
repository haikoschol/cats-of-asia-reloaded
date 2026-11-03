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

func TestPlausibleConfigurationValidation(t *testing.T) {
	valid := config{PlausibleEnabled: true, PlausibleDomain: "reloaded.example.test", PlausibleScriptURL: "https://analytics.example.test/js/script.js"}
	if err := valid.validatePlausible(); err != nil {
		t.Fatal(err)
	}
	// Disabled analytics must not require usable tracking settings.
	if err := (config{PlausibleDomain: "invalid/domain", PlausibleScriptURL: "invalid"}).validatePlausible(); err != nil {
		t.Fatal("disabled tracking required a domain or URL")
	}
	for _, domain := range []string{"", "https://reloaded.example.test", "reloaded.example.test/path", "reloaded.example.test:443", "one.example,two.example", "bad name.example", ".example.test", "bad-.example", strings.Repeat("a", 64) + ".example"} {
		t.Run("domain/"+domain, func(t *testing.T) {
			cfg := valid
			cfg.PlausibleDomain = domain
			if err := cfg.validatePlausible(); err == nil || !strings.Contains(err.Error(), "PLAUSIBLE_DOMAIN") {
				t.Fatal("invalid domain was accepted or produced an unrelated error")
			}
		})
	}
	for _, script := range []string{"", "/js/script.js", "//analytics.example.test/script.js", "http://analytics.example.test/script.js", "javascript:alert(1)", "https:///script.js", "https://user:private-example@analytics.example.test/script.js", "https://analytics.example.test/script.js#fragment", "https://analytics.example.test:invalid/script.js", "https://analytics.example.test:65536/script.js"} {
		t.Run("script/"+strings.ReplaceAll(script, "private-example", "redacted"), func(t *testing.T) {
			cfg := valid
			cfg.PlausibleScriptURL = script
			err := cfg.validatePlausible()
			if err == nil || !strings.Contains(err.Error(), "PLAUSIBLE_SCRIPT_URL") || strings.Contains(err.Error(), "private-example") {
				t.Fatal("invalid script URL was accepted or its value was exposed")
			}
		})
	}
	custom := valid
	custom.PlausibleScriptURL = "https://custom.example.test:8443/tracker.js?version=1"
	if err := custom.validatePlausible(); err != nil {
		t.Fatal("a valid custom HTTPS script URL was rejected")
	}
}
