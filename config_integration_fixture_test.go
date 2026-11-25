//go:build integration

package main

import "testing"

func setPrivateTestCredentials(t *testing.T) map[string]string {
	t.Helper()
	settings := map[string]string{
		"CLOUDFLARE_IMAGES_ACCOUNT_ID": "fake-private-cloudflare-account-id",
		"CLOUDFLARE_IMAGES_API_KEY":    "fake-private-cloudflare-key",
		"GOOGLE_MAPS_API_KEY":          "fake-private-google-key",
		"MASTODON_ACCESS_TOKEN":        "fake-private-mastodon-token",
	}
	for name, value := range settings {
		t.Setenv(name, value)
	}
	return settings
}
