//go:build !integration

package main

import (
	"strings"
	"testing"
)

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
