//go:build integration

package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadConfig(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	setPrivateTestCredentials(t)
	t.Setenv("DATABASE_URL", "postgres://example:example@localhost:5433/cats_test")
	t.Setenv("USERS_DATABASE_URL", "postgres://example:example@localhost:5433/users_test")
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "images.example.com")
	t.Setenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH", "example-account-hash")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "example-public-map-token")

	cfg, err := loadConfigFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	want := config{
		HTTPListenAddr:    ":3000",
		DatabaseURL:       "postgres://example:example@localhost:5433/cats_test",
		UsersDatabaseURL:  "postgres://example:example@localhost:5433/users_test",
		ImageDomain:       "images.example.com",
		ImageAccountHash:  "example-account-hash",
		ImageAccountID:    "fake-private-cloudflare-account-id",
		ImageAPIKey:       "fake-private-cloudflare-key",
		MapboxAccessToken: "example-public-map-token",
		GoogleMapsAPIKey:  "fake-private-google-key",
	}
	if cfg != want {
		t.Fatal("configuration did not match the example values")
	}

	// Database-only commands do not require map or image settings.
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "")
	t.Setenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH", "")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "")
	if _, err := loadConfigFile(".env"); err != nil {
		t.Fatalf("database-only configuration: %v", err)
	}

	for _, value := range []string{"", " \t\n"} {
		t.Setenv("DATABASE_URL", value)
		cfg, err := loadConfigFile(".env")
		if err != nil || cfg.DatabaseURL != want.UsersDatabaseURL {
			t.Fatal("blank DATABASE_URL did not fall back to USERS_DATABASE_URL")
		}
	}
}

func TestHTTPListenAddress(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://example:example@localhost/photos")
	t.Setenv("USERS_DATABASE_URL", "postgres://example:example@localhost/users")
	for _, address := range []string{":3000", "127.0.0.1:3001", "[::1]:3001", "localhost:3001"} {
		t.Setenv("HTTP_LISTEN_ADDR", " "+address+" ")
		cfg, err := loadConfigFile(".env")
		if err != nil || cfg.HTTPListenAddr != address {
			t.Fatal("valid HTTP listen address was rejected")
		}
	}
	for _, address := range []string{":0", ":65536", "127.0.0.1", "http://127.0.0.1:3001", "private-test-marker@host:3001"} {
		t.Setenv("HTTP_LISTEN_ADDR", address)
		_, err := loadConfigFile(".env")
		if err == nil || !strings.HasPrefix(err.Error(), "HTTP_LISTEN_ADDR must") || strings.Contains(err.Error(), "private-test-marker") {
			t.Fatal("invalid HTTP listen address was accepted or exposed")
		}
	}
}

func TestCloudflareUploadConfiguration(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://example:example@localhost/photos")
	t.Setenv("USERS_DATABASE_URL", "postgres://example:example@localhost/users")
	cfg, err := loadConfigFile(".env")
	if err != nil || cfg.ImageAccountID != "" || cfg.ImageAPIKey != "" {
		t.Fatal("upload credentials must be optional for startup")
	}
	if err := os.WriteFile(".env", []byte("CLOUDFLARE_IMAGES_ACCOUNT_ID=' aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa '\nCLOUDFLARE_IMAGES_API_KEY=' fake-private-cloudflare-key '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfigFile(".env")
	if err != nil || cfg.ImageAccountID != strings.Repeat("a", 32) || cfg.ImageAPIKey != "fake-private-cloudflare-key" {
		t.Fatal("private upload settings were not loaded and trimmed")
	}
	t.Setenv("CLOUDFLARE_IMAGES_API_KEY", "")
	cfg, err = loadConfigFile(".env")
	if err != nil || cfg.ImageAPIKey != "" {
		t.Fatal("explicit empty upload key did not override dotenv")
	}
}

func TestGoogleGeocodingConfiguration(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("DATABASE_URL=postgres://example:example@localhost/photos\nUSERS_DATABASE_URL=postgres://example:example@localhost/users\nGOOGLE_MAPS_API_KEY=' fake-private-google-key '\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(".env")
	if err != nil || cfg.GoogleGeocodingEnabled || cfg.GoogleMapsAPIKey != "fake-private-google-key" {
		t.Fatal("an existing key must be trimmed and leave geocoding disabled")
	}
	t.Setenv("GOOGLE_GEOCODING_ENABLED", "true")
	cfg, err = loadConfigFile(".env")
	if err != nil || !cfg.GoogleGeocodingEnabled {
		t.Fatal("explicit geocoding activation was not loaded")
	}
	t.Setenv("GOOGLE_MAPS_API_KEY", "")
	cfg, err = loadConfigFile(".env")
	if err != nil || cfg.GoogleMapsAPIKey != "" {
		t.Fatal("an explicitly empty environment key must override the file without blocking app startup")
	}
	t.Setenv("GOOGLE_GEOCODING_ENABLED", "invalid")
	if _, err := loadConfigFile(".env"); err == nil || err.Error() != "cannot parse environment configuration" {
		t.Fatal("an invalid activation setting must fail without exposing configuration")
	}
}

func TestStartupDoesNotLogSecrets(t *testing.T) {
	privateSettings := setPrivateTestCredentials(t)
	const contentURL = "postgres://reader:fake-private-db-password@content.example/photos"
	const usersURL = "postgres://writer:fake-private-db-password@users.example/users"
	tests := []struct{ name, databaseURL, usersURL, wantMessage, dotenv string }{
		{name: "missing users URL", databaseURL: contentURL, wantMessage: "USERS_DATABASE_URL is not set"},
		{name: "invalid content URL", databaseURL: "postgres://reader:fake-private-db-password@localhost:invalid/photos", usersURL: usersURL, wantMessage: "cannot parse DATABASE_URL"},
		{name: "invalid users URL", databaseURL: contentURL, usersURL: "postgres://writer:fake-private-db-password@localhost:invalid/users", wantMessage: "cannot parse USERS_DATABASE_URL"},
		{name: "invalid dotenv file", databaseURL: contentURL, usersURL: usersURL, dotenv: "CLOUDFLARE_IMAGES_API_KEY=\"fake-private-cloudflare-key\n", wantMessage: "cannot read .env"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("DATABASE_URL", tt.databaseURL)
			t.Setenv("USERS_DATABASE_URL", tt.usersURL)
			t.Setenv("CATS_CONFIG_STARTUP_TEST", "1")
			cmd := exec.Command(os.Args[0], "-test.run=^TestConfigStartupProcess$")
			cmd.Dir = t.TempDir()
			if tt.dotenv != "" {
				if err := os.WriteFile(filepath.Join(cmd.Dir, ".env"), []byte(tt.dotenv), 0o600); err != nil {
					t.Fatal(err)
				}
			}
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
			for _, connection := range []string{tt.databaseURL, tt.usersURL} {
				if connection != "" && strings.Contains(string(output), connection) {
					t.Error("startup log exposed a database connection URL")
				}
			}
		})
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"DATABASE_URL", "POSTGRES_USER", "POSTGRES_PASSWORD", "POSTGRES_DB",
		"USERS_DATABASE_URL",
		"HTTP_LISTEN_ADDR",
		"SESSION_COOKIE_SECURE",
		"PLAUSIBLE_ENABLED", "PLAUSIBLE_DOMAIN", "PLAUSIBLE_SCRIPT_URL",
		"CLOUDFLARE_IMAGES_DOMAIN", "CLOUDFLARE_IMAGES_ACCOUNT_HASH", "MAPBOX_ACCESS_TOKEN",
		"CLOUDFLARE_IMAGES_ACCOUNT_ID", "CLOUDFLARE_IMAGES_API_KEY",
		"GOOGLE_GEOCODING_ENABLED", "GOOGLE_MAPS_API_KEY",
	} {
		// t.Setenv records the original value so cleanup restores it after Unsetenv.
		t.Setenv(name, "")
		if err := os.Unsetenv(name); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLocalDotenvConfiguration(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	content := `POSTGRES_USER=sample
POSTGRES_PASSWORD='sample:@/?#password'
POSTGRES_DB=cats_sample
DATABASE_URL=' postgres://reader:example@photos.example.com/photos?sslmode=require '
USERS_DATABASE_URL=' postgres://writer:example@users.example.com/users?sslmode=require '
CLOUDFLARE_IMAGES_DOMAIN=images.example.com
CLOUDFLARE_IMAGES_ACCOUNT_HASH=sample-hash
MAPBOX_ACCESS_TOKEN=file-map-token
`
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ImageDomain != "images.example.com" || cfg.ImageAccountHash != "sample-hash" ||
		cfg.MapboxAccessToken != "file-map-token" {
		t.Fatal("image or map settings were not loaded from .env")
	}
	if cfg.DatabaseURL != "postgres://reader:example@photos.example.com/photos?sslmode=require" || cfg.UsersDatabaseURL != "postgres://writer:example@users.example.com/users?sslmode=require" {
		t.Fatal("database URLs were not loaded and trimmed")
	}
	if _, exists := os.LookupEnv("POSTGRES_PASSWORD"); exists {
		t.Fatal("loading .env changed the process environment")
	}

	// Explicit shell/deployment settings must win, including an empty value.
	t.Setenv("DATABASE_URL", "postgres://example:example@localhost:5433/cats_test")
	t.Setenv("MAPBOX_ACCESS_TOKEN", "environment-map-token")
	t.Setenv("CLOUDFLARE_IMAGES_DOMAIN", "")
	t.Setenv("USERS_DATABASE_URL", "postgres://writer:example@override.example/users")
	cfg, err = loadConfigFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DatabaseURL != os.Getenv("DATABASE_URL") || cfg.MapboxAccessToken != "environment-map-token" || cfg.ImageDomain != "" {
		t.Fatal(".env overwrote explicitly supplied environment values")
	}
	if cfg.UsersDatabaseURL != "postgres://writer:example@override.example/users" {
		t.Fatal(".env overwrote the explicit users URL")
	}
	t.Setenv("DATABASE_URL", "")
	cfg, err = loadConfigFile(".env")
	if err != nil || cfg.DatabaseURL != cfg.UsersDatabaseURL {
		t.Fatal("blank DATABASE_URL did not override the .env photo URL and fall back to the users URL")
	}
	t.Setenv("USERS_DATABASE_URL", "")
	if _, err := loadConfigFile(".env"); err == nil || err.Error() != "USERS_DATABASE_URL is not set" {
		t.Fatal("an explicitly empty users URL did not fail validation")
	}
}

func TestDatabaseURLInterpolation(t *testing.T) {
	clearConfigEnvironment(t)
	t.Chdir(t.TempDir())
	content := `POSTGRES_USER=sample
POSTGRES_PASSWORD=sample-password
POSTGRES_DB=cats_sample
USERS_DATABASE_URL=postgres://${POSTGRES_USER}:${POSTGRES_PASSWORD}@localhost:5432/${POSTGRES_DB}?sslmode=disable
`
	if err := os.WriteFile(".env", []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := loadConfigFile(".env")
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UsersDatabaseURL != "postgres://sample:sample-password@localhost:5432/cats_sample?sslmode=disable" {
		t.Fatal("explicit USERS_DATABASE_URL interpolation stopped working")
	}
	if cfg.DatabaseURL != cfg.UsersDatabaseURL {
		t.Fatal("omitting DATABASE_URL did not use the interpolated users URL")
	}
}

func TestSessionCookieConfiguration(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://reader:example@content.example/photos")
	t.Setenv("USERS_DATABASE_URL", "postgres://writer:example@localhost/users")
	path := filepath.Join(t.TempDir(), ".env")
	cfg, err := loadConfigFile(path)
	if err != nil || cfg.SessionCookieSecure {
		t.Fatalf("local HTTP default must work without a setting: %v", err)
	}
	if err := os.WriteFile(path, []byte("SESSION_COOKIE_SECURE=true\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err = loadConfigFile(path)
	if err != nil || !cfg.SessionCookieSecure {
		t.Fatalf("HTTPS setting was not loaded from .env: %v", err)
	}
	for _, value := range []string{"false", "true"} {
		t.Setenv("SESSION_COOKIE_SECURE", value)
		cfg, err = loadConfigFile(path)
		if err != nil || cfg.SessionCookieSecure != (value == "true") {
			t.Fatalf("explicit cookie setting did not override .env: %v", err)
		}
	}
	t.Setenv("SESSION_COOKIE_SECURE", "invalid-private-example")
	if _, err := loadConfigFile(path); err == nil || err.Error() != "cannot parse environment configuration" {
		t.Fatal("invalid cookie configuration did not produce a generic error")
	}
}

func TestUsersDatabaseURLMustBeExplicit(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("POSTGRES_USER", "sample")
	t.Setenv("POSTGRES_PASSWORD", "sample-password")
	t.Setenv("POSTGRES_DB", "cats_sample")
	t.Setenv("DATABASE_URL", "postgres://reader:example@content.example/photos")
	_, err := loadConfigFile(filepath.Join(t.TempDir(), ".env"))
	if err == nil || err.Error() != "USERS_DATABASE_URL is not set" {
		t.Fatal("users database URL was not required explicitly")
	}
}

func TestConfigStartupProcess(t *testing.T) {
	if os.Getenv("CATS_CONFIG_STARTUP_TEST") != "1" {
		t.Skip("only used by the startup log test")
	}
	os.Args = []string{"cats-of-asia-reloaded"}
	main()
}
