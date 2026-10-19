package main

import (
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/caarlos0/env/v11"
	"github.com/joho/godotenv"
)

type config struct {
	DatabaseURL       string `env:"DATABASE_URL"`
	UsersDatabaseURL  string `env:"USERS_DATABASE_URL"`
	ImageDomain       string `env:"CLOUDFLARE_IMAGES_DOMAIN"`
	ImageAccountHash  string `env:"CLOUDFLARE_IMAGES_ACCOUNT_HASH"`
	MapboxAccessToken string `env:"MAPBOX_ACCESS_TOKEN"`
	PostgresUser      string `env:"POSTGRES_USER"`
	PostgresPassword  string `env:"POSTGRES_PASSWORD"`
	PostgresDB        string `env:"POSTGRES_DB"`
}

// Read .env as data, without changing the process environment. Existing
// environment variables take precedence, including explicitly empty values.
func loadConfigFile(path string) (config, error) {
	settings, err := godotenv.Read(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			// Parser errors may contain lines from the file, including secrets.
			return config{}, fmt.Errorf("cannot read .env; check file permissions and syntax")
		}
		settings = make(map[string]string)
	}
	for _, assignment := range os.Environ() {
		name, value, _ := strings.Cut(assignment, "=")
		settings[name] = value
	}

	var cfg config
	if err := env.ParseWithOptions(&cfg, env.Options{Environment: settings}); err != nil {
		return config{}, fmt.Errorf("cannot parse environment configuration")
	}
	cfg.DatabaseURL = strings.TrimSpace(cfg.DatabaseURL)
	cfg.UsersDatabaseURL = strings.TrimSpace(cfg.UsersDatabaseURL)
	cfg.ImageDomain = strings.TrimSpace(cfg.ImageDomain)
	cfg.ImageAccountHash = strings.TrimSpace(cfg.ImageAccountHash)
	cfg.MapboxAccessToken = strings.TrimSpace(cfg.MapboxAccessToken)

	if cfg.DatabaseURL == "" {
		return config{}, fmt.Errorf("DATABASE_URL is not set")
	}
	if cfg.UsersDatabaseURL == "" {
		return config{}, fmt.Errorf("USERS_DATABASE_URL is not set")
	}
	return cfg, nil
}

func (cfg config) validateImages() error {
	if cfg.ImageDomain == "" {
		return fmt.Errorf("CLOUDFLARE_IMAGES_DOMAIN is not set")
	}
	if cfg.ImageAccountHash == "" {
		return fmt.Errorf("CLOUDFLARE_IMAGES_ACCOUNT_HASH is not set")
	}
	return nil
}

func (cfg config) validateMap() error {
	if cfg.MapboxAccessToken == "" {
		return fmt.Errorf("MAPBOX_ACCESS_TOKEN is not set")
	}
	if !strings.HasPrefix(cfg.MapboxAccessToken, "pk.") {
		return fmt.Errorf("MAPBOX_ACCESS_TOKEN must be a public browser token starting with pk.")
	}
	return nil
}

func (cfg config) validateLocalDatabase() error {
	for _, setting := range []struct{ name, value string }{
		{"POSTGRES_USER", cfg.PostgresUser},
		{"POSTGRES_PASSWORD", cfg.PostgresPassword},
		{"POSTGRES_DB", cfg.PostgresDB},
	} {
		if setting.value == "" {
			return fmt.Errorf("%s is not set", setting.name)
		}
	}
	return nil
}

func (cfg config) localDatabaseURL(port string) (string, error) {
	if err := cfg.validateLocalDatabase(); err != nil {
		return "", err
	}
	number, err := strconv.Atoi(port)
	if err != nil || number < 1 || number > 65535 {
		return "", fmt.Errorf("local database port must be between 1 and 65535")
	}
	connection := url.URL{
		Scheme:   "postgres",
		User:     url.UserPassword(cfg.PostgresUser, cfg.PostgresPassword),
		Host:     net.JoinHostPort("localhost", port),
		Path:     "/" + cfg.PostgresDB,
		RawQuery: "sslmode=disable",
	}
	return connection.String(), nil
}
