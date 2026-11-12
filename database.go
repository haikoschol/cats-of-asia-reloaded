package main

import (
	"database/sql"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func openContentDatabase(databaseURL string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		// Driver parse errors may include the original credential-bearing URL.
		return nil, errors.New("cannot parse DATABASE_URL")
	}
	cfg.RuntimeParams["default_transaction_read_only"] = "off"
	cfg.RuntimeParams["statement_timeout"] = "10000"
	cfg.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*cfg)
	db.SetMaxOpenConns(3)
	return db, nil
}

func openUsersDatabase(databaseURL string) (*sql.DB, error) {
	cfg, err := pgx.ParseConfig(databaseURL)
	if err != nil {
		return nil, errors.New("cannot parse USERS_DATABASE_URL")
	}
	cfg.ConnectTimeout = 5 * time.Second
	db := stdlib.OpenDB(*cfg)
	// Sessions and content can share one PostgreSQL role. Keep their combined
	// pools at six connections, below the test role's limit of eight.
	db.SetMaxOpenConns(3)
	return db, nil
}
