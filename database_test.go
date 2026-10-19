package main

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/alexedwards/scs/v2"
	"golang.org/x/crypto/bcrypt"
)

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	connection := os.Getenv("CATS_TEST_DATABASE_URL")
	if connection == "" || os.Getenv("DATABASE_URL") != connection || os.Getenv("USERS_DATABASE_URL") != connection {
		t.Fatal("run database tests with ./scripts/run-tests.sh; real database URLs must not be used")
	}
	return connection
}

func TestLoginUsesUsersDatabase(t *testing.T) {
	connection := testDatabaseURL(t)
	contentDB, err := sql.Open("pgx", connection)
	if err != nil {
		t.Fatal(err)
	}
	defer contentDB.Close()
	contentDB.SetMaxOpenConns(1)
	// Separate connection-local tables simulate independent databases without
	// touching persistent rows. The content side deliberately has no user.
	if _, err := contentDB.Exec(`
		CREATE TEMP TABLE users (LIKE public.users INCLUDING ALL);
		CREATE TEMP TABLE photos_location (LIKE public.photos_location INCLUDING ALL);
		INSERT INTO photos_location (city, country, tzoffset) VALUES ('Content place', 'Test country', 0);
	`); err != nil {
		t.Fatal(err)
	}
	usersDB, err := openUsersDatabase(connection)
	if err != nil {
		t.Fatal(err)
	}
	defer usersDB.Close()
	usersDB.SetMaxOpenConns(1)
	if _, err := usersDB.Exec("CREATE TEMP TABLE users (LIKE public.users INCLUDING ALL)"); err != nil {
		t.Fatal(err)
	}
	hash, err := bcrypt.GenerateFromPassword([]byte("test-password"), bcrypt.MinCost)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := usersDB.Exec("INSERT INTO users (username, hashed_password) VALUES ($1, $2)", "role-test-user", string(hash)); err != nil {
		t.Fatal(err)
	}
	app := &application{contentDB: contentDB, usersDB: usersDB, sessionManager: scs.New()}
	handler := app.routes()
	request := httptest.NewRequest(http.MethodPost, "/login", strings.NewReader("username=role-test-user&password=test-password"))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusSeeOther {
		t.Fatalf("login did not read the users database: status %d", response.Code)
	}
	homeRequest := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range response.Result().Cookies() {
		homeRequest.AddCookie(cookie)
	}
	homeResponse := httptest.NewRecorder()
	handler.ServeHTTP(homeResponse, homeRequest)
	if homeResponse.Code != http.StatusOK || !strings.Contains(homeResponse.Body.String(), "Log out") ||
		!strings.Contains(homeResponse.Body.String(), "Content place, Test country") {
		t.Fatal("homepage did not validate the user through usersDB and read places through contentDB")
	}
}
