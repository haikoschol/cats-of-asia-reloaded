package main

import (
	"bytes"
	"database/sql"
	"fmt"
	"os"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func createUser(db *sql.DB, username string) error {
	if username == "" {
		return fmt.Errorf("username must not be empty")
	}

	var exists bool
	err := db.QueryRow("SELECT EXISTS (SELECT 1 FROM users WHERE username = $1)", username).Scan(&exists)
	if err != nil {
		return fmt.Errorf("check whether username exists: %w", err)
	}
	if exists {
		return fmt.Errorf("username %q already exists", username)
	}

	password, err := readPassword("Password: ")
	if err != nil {
		return fmt.Errorf("read password: %w", err)
	}
	defer clearPassword(password)

	confirmation, err := readPassword("Confirm password: ")
	if err != nil {
		return fmt.Errorf("read password confirmation: %w", err)
	}
	defer clearPassword(confirmation)

	if len(password) == 0 {
		return fmt.Errorf("password must not be empty")
	}
	if !bytes.Equal(password, confirmation) {
		return fmt.Errorf("passwords do not match")
	}

	hashedPassword, err := bcrypt.GenerateFromPassword(password, bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}

	_, err = db.Exec(
		"INSERT INTO users (username, hashed_password) VALUES ($1, $2)",
		username,
		hashedPassword,
	)
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}

	fmt.Printf("Created user %q.\n", username)
	return nil
}

func readPassword(prompt string) ([]byte, error) {
	fmt.Print(prompt)
	password, err := term.ReadPassword(int(os.Stdin.Fd()))
	fmt.Println()
	return password, err
}

func clearPassword(password []byte) {
	for i := range password {
		password[i] = 0
	}
}
