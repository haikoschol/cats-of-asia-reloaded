//go:build integration

package main

import (
	"context"
	"testing"

	"github.com/alexedwards/scs/v2"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"
)

func TestAuthenticatedUserIDDatabaseError(t *testing.T) {
	db := stdlib.OpenDB(pgx.ConnConfig{})
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	sessions := scs.New()
	ctx, err := sessions.Load(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	sessions.Put(ctx, "authenticatedUserID", 1)
	app := &application{usersDB: db, sessionManager: sessions}
	userID, err := app.authenticatedUserID(ctx)
	if err == nil || userID != 0 {
		t.Fatalf("database failure must not authenticate: ID %d, error %v", userID, err)
	}
	if sessions.GetInt(ctx, "authenticatedUserID") != 1 {
		t.Fatal("database failure discarded the session")
	}
}
