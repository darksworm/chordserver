package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/mattn/go-sqlite3"
)

// ChordData represents the structure of your input JSON files
type ChordData struct {
	Key       string     `json:"key"`
	Suffix    string     `json:"suffix"`
	Positions []Position `json:"positions"`
}

// Position represents a single chord position/fingering
type Position struct {
	Frets   string `json:"frets"`
	Fingers string `json:"fingers"`
	Barres  string `json:"barres,omitempty"`
	Capo    string `json:"capo,omitempty"`
}

func main() {
	sourceDir := flag.String("source", "", "Source directory containing chord JSON files")
	outputFile := flag.String("output", "chords.db", "Output SQLite database file")
	flag.Parse()

	if *sourceDir == "" {
		fmt.Println("Usage: go run script.go -source=/path/to/source [-output=chords.db]")
		os.Exit(1)
	}

	// Remove existing database if it exists
	if _, err := os.Stat(*outputFile); err == nil {
		if err := os.Remove(*outputFile); err != nil {
			fmt.Printf("Error removing existing database: %v\n", err)
			os.Exit(1)
		}
	}

	// Create and open database
	db, err := sql.Open("sqlite3", *outputFile)
	if err != nil {
		fmt.Printf("Error opening database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create tables
	createTables(db)

	// Prepare insert statements
	chordStmt, err := db.Prepare(`
		INSERT INTO chords (key, suffix, full_data) 
		VALUES (?, ?, ?)
	`)
	if err != nil {
		fmt.Printf("Error preparing chord statement: %v\n", err)
		os.Exit(1)
	}
	defer chordStmt.Close()

	fingStmt, err := db.Prepare(`
		INSERT INTO fingerings (chord_id, frets, fingers, barres, capo) 
		VALUES (?, ?, ?, ?, ?)
	`)
	if err != nil {
		fmt.Printf("Error preparing fingering statement: %v\n", err)
		os.Exit(1)
	}
	defer fingStmt.Close()

	aliasStmt, err := db.Prepare(`
		INSERT INTO chord_aliases (chord_id, alias_key, alias_suffix) 
		VALUES (?, ?, ?)
	`)
	if err != nil {
		fmt.Printf("Error preparing alias statement: %v\n", err)
		os.Exit(1)
	}
	defer aliasStmt.Close()

	// Start transaction for bulk insertion
	tx, err := db.Begin()
	if err != nil {
		fmt.Printf("Error starting transaction: %v\n", err)
		os.Exit(1)
	}

	// Counters
	chordCount := 0
	fingeringCount := 0
	aliasCount := 0

	// Process all files
	err = filepath.Walk(*sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories and non-JSON files
		if info.IsDir() || !strings.HasSuffix(info.Name(), ".json") {
			return nil
		}

		// Read the file
		data, err := ioutil.ReadFile(path)
		if err != nil {
			fmt.Printf("Error reading file %s: %v\n", path, err)
			return nil
		}

		// Parse the JSON
		var chordData ChordData
		if err := json.Unmarshal(data, &chordData); err != nil {
			fmt.Printf("Error parsing JSON from %s: %v\n", path, err)
			return nil
		}

		// Insert the chord
		res, err := tx.Stmt(chordStmt).Exec(
			chordData.Key,
			chordData.Suffix,
			string(data),
		)
		if err != nil {
			fmt.Printf("Error inserting chord: %v\n", err)
			return nil
		}

		// Get the chord ID
		chordID, err := res.LastInsertId()
		if err != nil {
			fmt.Printf("Error getting last insert ID: %v\n", err)
			return nil
		}
		chordCount++

		// Insert fingerings
		for _, pos := range chordData.Positions {
			_, err := tx.Stmt(fingStmt).Exec(
				chordID,
				pos.Frets,
				pos.Fingers,
				pos.Barres,
				pos.Capo,
			)
			if err != nil {
				fmt.Printf("Error inserting fingering: %v\n", err)
				continue
			}
			fingeringCount++
		}

		// Insert aliases
		key := chordData.Key
		suffix := chordData.Suffix

		// Generate aliases for the suffix
		suffixAliases := getSuffixAliases(suffix)

		// Insert aliases
		for _, aliasStr := range suffixAliases {
			// Skip if it's the same as the original
			if aliasStr == suffix {
				continue
			}

			_, err := tx.Stmt(aliasStmt).Exec(
				chordID,
				key,
				aliasStr,
			)
			if err != nil {
				fmt.Printf("Error inserting alias: %v\n", err)
				continue
			}
			aliasCount++
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		tx.Rollback()
		os.Exit(1)
	}

	// Commit the transaction
	if err := tx.Commit(); err != nil {
		fmt.Printf("Error committing transaction: %v\n", err)
		os.Exit(1)
	}

	// Create indexes after inserting data (faster)
	createIndexes(db)

	// Optimize database
	_, err = db.Exec("VACUUM;")
	if err != nil {
		fmt.Printf("Error optimizing database: %v\n", err)
	}

	// Output stats
	fmt.Println("Database creation complete!")
	fmt.Printf("Generated SQLite database at %s\n", *outputFile)
	fmt.Printf("Inserted %d chords\n", chordCount)
	fmt.Printf("Inserted %d fingerings\n", fingeringCount)
	fmt.Printf("Created %d chord aliases\n", aliasCount)

	// Output file size
	fileInfo, err := os.Stat(*outputFile)
	if err == nil {
		fmt.Printf("Database size: %.2f MB\n", float64(fileInfo.Size())/(1024*1024))
	}
}

// Create database tables
func createTables(db *sql.DB) {
	// Create chords table
	_, err := db.Exec(`
		CREATE TABLE chords (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			key TEXT NOT NULL,
			suffix TEXT NOT NULL,
			full_data TEXT NOT NULL,
			UNIQUE(key, suffix)
		);
	`)
	if err != nil {
		fmt.Printf("Error creating chords table: %v\n", err)
		os.Exit(1)
	}

	// Create fingerings table
	_, err = db.Exec(`
		CREATE TABLE fingerings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chord_id INTEGER NOT NULL,
			frets TEXT NOT NULL,
			fingers TEXT,
			barres TEXT,
			capo TEXT,
			FOREIGN KEY(chord_id) REFERENCES chords(id)
		);
	`)
	if err != nil {
		fmt.Printf("Error creating fingerings table: %v\n", err)
		os.Exit(1)
	}

	// Create chord aliases table
	_, err = db.Exec(`
		CREATE TABLE chord_aliases (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			chord_id INTEGER NOT NULL,
			alias_key TEXT NOT NULL,
			alias_suffix TEXT NOT NULL,
			UNIQUE(alias_key, alias_suffix),
			FOREIGN KEY(chord_id) REFERENCES chords(id)
		);
	`)
	if err != nil {
		fmt.Printf("Error creating chord_aliases table: %v\n", err)
		os.Exit(1)
	}
}

// Create indexes for faster querying
func createIndexes(db *sql.DB) {
	// Index for chord lookup by key+suffix
	_, err := db.Exec(`CREATE INDEX idx_chords_key_suffix ON chords(key, suffix);`)
	if err != nil {
		fmt.Printf("Error creating index on chords: %v\n", err)
	}

	// Index for fingering lookup
	_, err = db.Exec(`CREATE INDEX idx_fingerings_frets ON fingerings(frets);`)
	if err != nil {
		fmt.Printf("Error creating index on fingerings: %v\n", err)
	}

	// Index for chord_id in fingerings for faster joins
	_, err = db.Exec(`CREATE INDEX idx_fingerings_chord_id ON fingerings(chord_id);`)
	if err != nil {
		fmt.Printf("Error creating index on fingerings chord_id: %v\n", err)
	}

	// Index for alias lookup
	_, err = db.Exec(`CREATE INDEX idx_aliases_key_suffix ON chord_aliases(alias_key, alias_suffix);`)
	if err != nil {
		fmt.Printf("Error creating index on aliases: %v\n", err)
	}
}

// getSuffixAliases returns a list of aliases for a given chord suffix
func getSuffixAliases(suffix string) []string {
	suffix = strings.TrimSpace(suffix)

	// Always include the original suffix
	aliases := []string{suffix}

	// Handle special cases
	switch strings.ToLower(suffix) {
	case "", "major":
		aliases = append(aliases, "major", "maj", "M", "")
	case "minor":
		aliases = append(aliases, "minor", "min", "m")
	case "5":
		aliases = append(aliases, "5", "power", "fifth")
	case "7":
		aliases = append(aliases, "7", "dominant7", "dom7")
	case "m7", "min7":
		aliases = append(aliases, "m7", "min7", "minor7")
	case "maj7":
		aliases = append(aliases, "maj7", "major7", "M7")
	case "sus2":
		aliases = append(aliases, "sus2", "suspended2")
	case "sus4":
		aliases = append(aliases, "sus4", "suspended4")
	}

	// Return unique aliases
	uniqueAliases := make(map[string]bool)
	for _, a := range aliases {
		uniqueAliases[a] = true
	}

	result := []string{}
	for a := range uniqueAliases {
		result = append(result, a)
	}

	return result
}
