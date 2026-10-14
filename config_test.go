package main

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	setPrivateTestCredentials(t)
	t.Setenv("DATABASE_URL", "postgres://example:example@localhost:5433/cats_test")
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "images.example.com")
	t.Setenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH", "example-account-hash")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "example-public-map-token")

	cfg, err := loadConfig()
	if err != nil {
		t.Fatal(err)
	}
	want := config{
		databaseURL:       "postgres://example:example@localhost:5433/cats_test",
		imageDomain:       "images.example.com",
		imageAccountHash:  "example-account-hash",
		mapboxAccessToken: "example-public-map-token",
	}
	if cfg != want {
		t.Fatal("configuration did not match the example values")
	}

	// Creating an account and running the current homepage need only the database.
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "")
	t.Setenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH", "")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "")
	if _, err := loadConfig(); err != nil {
		t.Fatalf("database-only configuration: %v", err)
	}

	for _, value := range []string{"", " \t\n"} {
		t.Setenv("DATABASE_URL", value)
		if _, err := loadConfig(); err == nil || err.Error() != "DATABASE_URL is not set" {
			t.Fatalf("wanted a missing DATABASE_URL error, got %v", err)
		}
	}
}

func TestConfigFeatureRequirements(t *testing.T) {
	setPrivateTestCredentials(t)
	tests := []struct {
		name    string
		cfg     config
		images  bool
		wantErr string
	}{
		{name: "missing image domain", images: true,
			cfg:     config{imageAccountHash: "example-account-hash"},
			wantErr: "CLOUDFLARE_IMAGES_DOMAIN is not set"},
		{name: "missing image account hash", images: true,
			cfg:     config{imageDomain: "images.example.com"},
			wantErr: "CLOUDFLARE_IMAGES_ACCOUNT_HASH is not set"},
		{name: "images without a map token", images: true,
			cfg: config{imageDomain: "images.example.com", imageAccountHash: "example-account-hash"}},
		{name: "missing map token", wantErr: "MAPBOX_ACCESS_TOKEN is not set"},
		{name: "map without image settings", cfg: config{mapboxAccessToken: "example-public-map-token"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.databaseURL = "postgres://example:fake-private-db-password@localhost/cats"
			var err error
			if tt.images {
				err = tt.cfg.validateImages()
			} else {
				err = tt.cfg.validateMap()
			}
			if tt.wantErr == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || err.Error() != tt.wantErr {
				t.Fatalf("wanted %q, got %v", tt.wantErr, err)
			}
		})
	}
}

func setPrivateTestCredentials(t *testing.T) map[string]string {
	t.Helper()
	settings := map[string]string{
		"POSTGRES_PASSWORD":            "fake-private-db-password",
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

func TestStartupDoesNotLogSecrets(t *testing.T) {
	privateSettings := setPrivateTestCredentials(t)
	tests := []struct {
		name        string
		databaseURL string
		wantMessage string
	}{
		{name: "missing database URL", wantMessage: "DATABASE_URL is not set"},
		{name: "invalid database URL",
			databaseURL: "postgres://example:fake-private-db-password@localhost:invalid/cats",
			wantMessage: "invalid port"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("CATS_CONFIG_STARTUP_TEST", "1")
			// Run main in a child process because log.Fatal exits the process.
			cmd := exec.Command(os.Args[0], "-test.run=^TestConfigStartupProcess$")
			output, err := cmd.CombinedOutput()
			if err == nil {
				t.Fatal("startup succeeded with missing or invalid database configuration")
			}
			if !strings.Contains(string(output), tt.wantMessage) {
				t.Errorf("startup log did not contain %q", tt.wantMessage)
			}
			for name, value := range privateSettings {
				if strings.Contains(string(output), value) {
					t.Errorf("startup log exposed the value of %s", name)
				}
			}
			if tt.databaseURL != "" && strings.Contains(string(output), tt.databaseURL) {
				t.Error("startup log exposed the database connection URL")
			}
		})
	}
}

func TestConfigStartupProcess(t *testing.T) {
	if os.Getenv("CATS_CONFIG_STARTUP_TEST") != "1" {
		t.Skip("only used by the startup log test")
	}
	os.Args = []string{"cats-of-asia-reloaded"}
	main()
}
