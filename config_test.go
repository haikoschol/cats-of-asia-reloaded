package main

import (
	"net/url"
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
		DatabaseURL:       "postgres://example:example@localhost:5433/cats_test",
		UsersDatabaseURL:  "postgres://example:example@localhost:5433/users_test",
		ImageDomain:       "images.example.com",
		ImageAccountHash:  "example-account-hash",
		MapboxAccessToken: "example-public-map-token",
		PostgresPassword:  "fake-private-db-password",
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
		if _, err := loadConfigFile(".env"); err == nil || err.Error() != "DATABASE_URL is not set" {
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
	const contentURL = "postgres://reader:fake-private-db-password@content.example/photos"
	const usersURL = "postgres://writer:fake-private-db-password@users.example/users"
	tests := []struct{ name, databaseURL, usersURL, wantMessage, dotenv string }{
		{name: "missing content URL", usersURL: usersURL, wantMessage: "DATABASE_URL is not set"},
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
		"CLOUDFLARE_IMAGES_DOMAIN", "CLOUDFLARE_IMAGES_ACCOUNT_HASH", "MAPBOX_ACCESS_TOKEN",
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
	localURL, err := cfg.localDatabaseURL("5432")
	if err != nil {
		t.Fatal(err)
	}
	connection, err := url.Parse(localURL)
	if err != nil {
		t.Fatal("could not parse the derived database URL")
	}
	password, _ := connection.User.Password()
	if connection.Host != "localhost:5432" || connection.Path != "/cats_sample" ||
		connection.User.Username() != "sample" || password != "sample:@/?#password" {
		t.Fatal("derived connection did not preserve the local database settings")
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
DATABASE_URL=postgres://reader:example@content.example/photos
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

func TestDatabaseURLsMustBeExplicit(t *testing.T) {
	for _, missing := range []string{"DATABASE_URL", "USERS_DATABASE_URL"} {
		t.Run(missing, func(t *testing.T) {
			clearConfigEnvironment(t)
			t.Setenv("POSTGRES_USER", "sample")
			t.Setenv("POSTGRES_PASSWORD", "sample-password")
			t.Setenv("POSTGRES_DB", "cats_sample")
			t.Setenv("DATABASE_URL", "postgres://reader:example@content.example/photos")
			t.Setenv("USERS_DATABASE_URL", "postgres://writer:example@localhost/users")
			t.Setenv(missing, " \t\n")
			_, err := loadConfigFile(filepath.Join(t.TempDir(), ".env"))
			if err == nil || err.Error() != missing+" is not set" {
				t.Fatal("database URL was not required explicitly")
			}
		})
	}
}

func TestConfigStartupProcess(t *testing.T) {
	if os.Getenv("CATS_CONFIG_STARTUP_TEST") != "1" {
		t.Skip("only used by the startup log test")
	}
	os.Args = []string{"cats-of-asia-reloaded"}
	if os.Getenv("CATS_CONFIG_HELPER_COMMAND") == "prepare-local" {
		os.Args = append(os.Args, "prepare-local")
	}
	main()
}
