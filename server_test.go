package main

import (
	"database/sql"
	"os"
	"testing"

	_ "github.com/mattn/go-sqlite3"
)

func TestDatabaseConnection(t *testing.T) {
	// Check if the database file exists
	dbPath := "./chords.db"
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Fatalf("Database file %s does not exist", dbPath)
	}

	// Try to open the database
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("Failed to open database: %v", err)
	}
	defer db.Close()

	// Ping the database to verify connection
	if err := db.Ping(); err != nil {
		t.Fatalf("Failed to ping database: %v", err)
	}

	// Check if the chords table exists and has data
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM chords").Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query chords table: %v", err)
	}

	if count == 0 {
		t.Errorf("Chords table is empty")
	} else {
		t.Logf("Chords table has %d entries", count)
	}
}
