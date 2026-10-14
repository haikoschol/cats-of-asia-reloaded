package main

import (
	"fmt"
	"os"
	"strings"
)

type config struct {
	databaseURL       string
	imageDomain       string
	imageAccountHash  string
	mapboxAccessToken string
}

// Load settings once at startup. Image and map settings are validated when
// those features are used, so database-only commands do not require them.
func loadConfig() (config, error) {
	cfg := config{
		databaseURL:       strings.TrimSpace(os.Getenv("DATABASE_URL")),
		imageDomain:       strings.TrimSpace(os.Getenv("CLOUDFLARE_IMAGES_DOMAIN")),
		imageAccountHash:  strings.TrimSpace(os.Getenv("CLOUDFLARE_IMAGES_ACCOUNT_HASH")),
		mapboxAccessToken: strings.TrimSpace(os.Getenv("MAPBOX_ACCESS_TOKEN")),
	}
	if cfg.databaseURL == "" {
		return config{}, fmt.Errorf("DATABASE_URL is not set")
	}
	return cfg, nil
}

func (cfg config) validateImages() error {
	if cfg.imageDomain == "" {
		return fmt.Errorf("CLOUDFLARE_IMAGES_DOMAIN is not set")
	}
	if cfg.imageAccountHash == "" {
		return fmt.Errorf("CLOUDFLARE_IMAGES_ACCOUNT_HASH is not set")
	}
	return nil
}

func (cfg config) validateMap() error {
	if cfg.mapboxAccessToken == "" {
		return fmt.Errorf("MAPBOX_ACCESS_TOKEN is not set")
	}
	return nil
}
