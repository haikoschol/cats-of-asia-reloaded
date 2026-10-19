package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareLocalUsesDotenv(t *testing.T) {
	clearConfigEnvironment(t)
	dir := t.TempDir()
	content := "POSTGRES_USER=sample\nPOSTGRES_PASSWORD='fake-private-password'\nPOSTGRES_DB=cats_sample\nDATABASE_URL=postgres://reader:example@content.example/photos\nUSERS_DATABASE_URL=postgres://sample:example@localhost/cats_sample\n"
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CATS_CONFIG_STARTUP_TEST", "1")
	t.Setenv("CATS_CONFIG_HELPER_COMMAND", "prepare-local")
	command := exec.Command(os.Args[0], "-test.run=^TestConfigStartupProcess$")
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatal("prepare-local did not load .env before the database was running")
	}
	if strings.Contains(string(output), "fake-private-password") {
		t.Fatal("prepare-local printed the database password")
	}
	data, err := os.ReadFile(filepath.Join(dir, "pgconsole.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `database = "cats_sample"`) ||
		!strings.Contains(string(data), `password = "fake-private-password"`) {
		t.Fatal("pgconsole configuration did not contain the local file settings")
	}
}

func TestPgconsoleCredentialsAreQuotedAndProtected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pgconsole.toml")
	if err := os.WriteFile(path, []byte("old configuration"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config{
		PostgresUser:     "sample",
		PostgresDB:       "cats_sample",
		PostgresPassword: "fake-private-\"password\\\n[[injected]]\t猫",
	}
	if err := writePgconsoleConfig(cfg, path); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatal("pgconsole file is readable by other users")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "password = ") {
			var password string
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "password = ")), &password); err != nil {
				t.Fatal("password was not safely quoted")
			}
			if password != cfg.PostgresPassword {
				t.Fatal("quoting changed the password")
			}
			return
		}
	}
	t.Fatal("password entry was missing")
}

func TestRunTestsUsesIsolatedConnection(t *testing.T) {
	dir := t.TempDir()
	// Replace go only for this test. Check the child environment without connecting
	// to any database or recursively starting the real test suite.
	program := `#!/bin/sh
[ "$DATABASE_URL" = "$EXPECTED_TEST_DATABASE_URL" ] || exit 1
[ "$USERS_DATABASE_URL" = "$EXPECTED_TEST_DATABASE_URL" ] || exit 1
[ "$CATS_TEST_DATABASE_URL" = "$EXPECTED_TEST_DATABASE_URL" ] || exit 1
[ "$1" = test ] && [ "$2" = -count=1 ] && [ "$3" = ./... ]
`
	if err := os.WriteFile(filepath.Join(dir, "go"), []byte(program), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DATABASE_URL", "postgres://example:fake-private-password@remote.example/cats")
	t.Setenv("USERS_DATABASE_URL", "postgres://reader:fake-private-password@remote.example/photos")
	t.Setenv("EXPECTED_TEST_DATABASE_URL", "postgres://sample:sample-password@localhost:5433/cats_sample?sslmode=disable")
	cfg := config{PostgresUser: "sample", PostgresPassword: "sample-password", PostgresDB: "cats_sample"}
	if err := runTests(cfg, "5433"); err != nil {
		t.Fatal("tests did not use the isolated local connection")
	}
	if os.Getenv("DATABASE_URL") != "postgres://example:fake-private-password@remote.example/cats" {
		t.Fatal("test runner overwrote its parent's database connection")
	}
	if os.Getenv("USERS_DATABASE_URL") != "postgres://reader:fake-private-password@remote.example/photos" {
		t.Fatal("test runner overwrote its parent's users connection")
	}
}
