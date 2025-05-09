package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	_ "github.com/mattn/go-sqlite3"
)

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Set CORS headers
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		// Handle preflight requests
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		// Call the next handler
		next.ServeHTTP(w, r)
	})
}

var db *sql.DB

func main() {
	var err error
	db, err = sql.Open("sqlite3", "chords.db")
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}
	defer db.Close()

	// Create a new mux
	mux := http.NewServeMux()

	// Route handlers
	mux.HandleFunc("/chords/", getChordByName)
	mux.HandleFunc("/fingers/", getChordsByFingering)

	// Apply CORS middleware
	handler := corsMiddleware(mux)

	// Start server
	fmt.Println("Server running on http://localhost:8080")
	log.Fatal(http.ListenAndServe(":8080", handler))
}

func getChordByName(w http.ResponseWriter, r *http.Request) {
	// Extract chord name from URL
	chordPath := r.URL.Path[len("/chord/")+1:]
	if chordPath == "" {
		http.Error(w, "Chord name required", http.StatusBadRequest)
		return
	}

	// Prepare response
	w.Header().Set("Content-Type", "application/json")

	// Check if it contains a slash character, which would indicate a slash chord
	var key, suffix string
	for i, c := range chordPath {
		if !((c >= 'A' && c <= 'G') || c == '#' || c == 'b') {
			key = chordPath[:i]
			suffix = chordPath[i:]
			break
		}
	}
	if key == "" {
		key = chordPath
		suffix = ""
	}

	// First try direct lookup
	var fullData string
	err := db.QueryRow(`
		SELECT full_data FROM chords 
		WHERE key = ? AND suffix = ?
	`, key, suffix).Scan(&fullData)

	// If not found, try alias lookup
	if err == sql.ErrNoRows {
		err = db.QueryRow(`
			SELECT c.full_data 
			FROM chords c
			JOIN chord_aliases a ON c.id = a.chord_id
			WHERE a.alias_key = ? AND a.alias_suffix = ?
		`, key, suffix).Scan(&fullData)
	}

	if err == sql.ErrNoRows {
		http.Error(w, "Chord not found", http.StatusNotFound)
		return
	} else if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("DB error: %v", err)
		return
	}

	// Return the full JSON
	fmt.Fprint(w, fullData)
}

func getChordsByFingering(w http.ResponseWriter, r *http.Request) {
	// Extract fingering pattern from URL
	fingering := r.URL.Path[len("/fingering/"):]
	if fingering == "" {
		http.Error(w, "Fingering pattern required", http.StatusBadRequest)
		return
	}

	// Prepare response
	w.Header().Set("Content-Type", "application/json")

	// Query the database
	rows, err := db.Query(`
		SELECT c.full_data 
		FROM chords c
		JOIN fingerings f ON c.id = f.chord_id
		WHERE f.frets = ?
	`, fingering)

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("DB error: %v", err)
		return
	}
	defer rows.Close()

	// Collect results
	var results []json.RawMessage
	for rows.Next() {
		var fullData string
		if err := rows.Scan(&fullData); err != nil {
			http.Error(w, "Error reading results", http.StatusInternalServerError)
			log.Printf("Row scan error: %v", err)
			return
		}
		results = append(results, json.RawMessage(fullData))
	}

	if len(results) == 0 {
		http.Error(w, "No chords found with this fingering", http.StatusNotFound)
		return
	}

	// Return the results as JSON array
	response, err := json.Marshal(results)
	if err != nil {
		http.Error(w, "Error encoding response", http.StatusInternalServerError)
		return
	}

	fmt.Fprint(w, string(response))
}
