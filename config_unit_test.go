//go:build !integration

package main

import (
	"testing"
)

func TestConfigFeatureRequirements(t *testing.T) {
	tests := []struct {
		name    string
		cfg     config
		images  bool
		wantErr string
	}{
		{name: "missing image domain", images: true,
			cfg:     config{ImageAccountHash: "example-account-hash"},
			wantErr: "CLOUDFLARE_IMAGES_DOMAIN is not set"},
		{name: "missing image account hash", images: true,
			cfg:     config{ImageDomain: "images.example.com"},
			wantErr: "CLOUDFLARE_IMAGES_ACCOUNT_HASH is not set"},
		{name: "images without a map token", images: true,
			cfg: config{ImageDomain: "images.example.com", ImageAccountHash: "example-account-hash"}},
		{name: "missing map token", wantErr: "MAPBOX_ACCESS_TOKEN is not set"},
		{name: "map without image settings", cfg: config{MapboxAccessToken: "pk.example-public-map-token"}},
		{name: "reject secret map token", cfg: config{MapboxAccessToken: "sk.fake-secret-token"}, wantErr: "MAPBOX_ACCESS_TOKEN must be a public browser token starting with pk."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.cfg.DatabaseURL = "postgres://example:fake-private-db-password@localhost/cats"
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

func TestLocalDatabaseRequirements(t *testing.T) {
	for _, setting := range []string{"POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB"} {
		t.Run(setting, func(t *testing.T) {
			cfg := config{PostgresUser: "sample", PostgresPassword: "sample-password", PostgresDB: "cats_sample"}
			switch setting {
			case "POSTGRES_USER":
				cfg.PostgresUser = ""
			case "POSTGRES_PASSWORD":
				cfg.PostgresPassword = ""
			case "POSTGRES_DB":
				cfg.PostgresDB = ""
			}
			if _, err := cfg.localDatabaseURL("5433"); err == nil || err.Error() != setting+" is not set" {
				t.Fatal("missing local database setting did not produce its named error")
			}
		})
	}
	for _, port := range []string{"", "invalid", "0", "65536"} {
		cfg := config{PostgresUser: "sample", PostgresPassword: "sample-password", PostgresDB: "cats_sample"}
		if _, err := cfg.localDatabaseURL(port); err == nil {
			t.Error("invalid local database port was accepted")
		}
	}
}
