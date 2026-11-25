//go:build !integration

package main

import (
	"context"
	"testing"

	"github.com/alexedwards/scs/v2"
)

func TestAuthenticatedUserIDAnonymousAndMalformed(t *testing.T) {
	for _, tt := range []struct {
		name  string
		value any
	}{
		{name: "anonymous"},
		{name: "zero", value: 0},
		{name: "negative", value: -1},
		{name: "string", value: "1"},
		{name: "wrong numeric type", value: int64(1)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			sessions := scs.New()
			ctx, err := sessions.Load(context.Background(), "")
			if err != nil {
				t.Fatal(err)
			}
			if tt.value != nil {
				sessions.Put(ctx, "authenticatedUserID", tt.value)
			}
			// No database is needed when there is no valid stored user ID.
			app := &application{sessionManager: sessions}
			userID, err := app.authenticatedUserID(ctx)
			if err != nil || userID != 0 || sessions.Exists(ctx, "authenticatedUserID") {
				t.Fatalf("invalid session was not cleared: ID %d, error %v", userID, err)
			}
		})
	}
}
