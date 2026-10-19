package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

func writePgconsoleConfig(cfg config, path string) error {
	if err := cfg.validateLocalDatabase(); err != nil {
		return err
	}
	// JSON-quoted strings also work as TOML basic strings. This preserves quotes,
	// backslashes and control characters in credentials without injecting TOML.
	quote := func(value string) string {
		encoded, _ := json.Marshal(value)
		return string(encoded)
	}
	content := fmt.Sprintf(`[[connections]]
id = "cats"
name = "Cats of Asia local"
host = "db"
port = 5432
database = %s
username = %s
password = %s
`, quote(cfg.PostgresDB), quote(cfg.PostgresUser), quote(cfg.PostgresPassword))

	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("cannot create pgconsole.toml")
	}
	defer file.Close()
	if err := file.Chmod(0o600); err != nil {
		return fmt.Errorf("cannot protect pgconsole.toml permissions")
	}
	if _, err := file.WriteString(content); err != nil {
		return fmt.Errorf("cannot write pgconsole.toml")
	}
	return nil
}

func runTests(cfg config, port string) error {
	// Override both roles: tests must never connect to configured real databases.
	databaseURL, err := cfg.localDatabaseURL(port)
	if err != nil {
		return err
	}
	command := exec.Command("go", "test", "-count=1", "./...")
	for _, assignment := range os.Environ() {
		if !strings.HasPrefix(assignment, "DATABASE_URL=") &&
			!strings.HasPrefix(assignment, "USERS_DATABASE_URL=") &&
			!strings.HasPrefix(assignment, "CATS_TEST_DATABASE_URL=") {
			command.Env = append(command.Env, assignment)
		}
	}
	command.Env = append(command.Env, "DATABASE_URL="+databaseURL)
	command.Env = append(command.Env, "USERS_DATABASE_URL="+databaseURL)
	command.Env = append(command.Env, "CATS_TEST_DATABASE_URL="+databaseURL)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		return fmt.Errorf("Go tests failed")
	}
	return nil
}
