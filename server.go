package main

import (
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"strings"

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
	// Parse command line flags
	port := flag.Int("port", 8080, "Port to run the server on")
	flag.Parse()

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
	mux.HandleFunc("/search/", searchChords)

	// Apply CORS middleware
	handler := corsMiddleware(mux)

	// Start server
	addr := fmt.Sprintf(":%d", *port)
	fmt.Printf("Server running on http://localhost%s\n", addr)
	log.Fatal(http.ListenAndServe(addr, handler))
}

func getChordByName(w http.ResponseWriter, r *http.Request) {
	// Extract chord name from URL
	chordPath := r.URL.Path[len("/chords/"):]
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

	// Handle special cases for flat and sharp notations
	alternateKeys := []string{key}

	// Map flat notations to sharp equivalents
	if len(key) == 2 && key[1] == 'b' {
		switch key[0] {
		case 'A':
			alternateKeys = append(alternateKeys, "G#")
		case 'B':
			alternateKeys = append(alternateKeys, "A#")
		case 'C':
			alternateKeys = append(alternateKeys, "B")
		case 'D':
			alternateKeys = append(alternateKeys, "C#")
		case 'E':
			alternateKeys = append(alternateKeys, "D#")
		case 'F':
			alternateKeys = append(alternateKeys, "E")
		case 'G':
			alternateKeys = append(alternateKeys, "F#")
		}
	}

	// Handle special enharmonic equivalents
	if key == "B#" {
		alternateKeys = append(alternateKeys, "C")
	} else if key == "E#" {
		alternateKeys = append(alternateKeys, "F")
	}

	// Define common suffix aliases
	suffixVariants := []string{suffix}

	// Add common aliases based on the suffix
	switch strings.ToLower(suffix) {
	case "", "major":
		suffixVariants = append(suffixVariants, "major", "maj", "M", "")
	case "minor", "min", "m":
		suffixVariants = append(suffixVariants, "minor", "min", "m")
	case "5":
		suffixVariants = append(suffixVariants, "5", "power", "fifth")
	case "7":
		suffixVariants = append(suffixVariants, "7", "dominant7", "dom7")
	case "m7", "min7", "minor7":
		suffixVariants = append(suffixVariants, "m7", "min7", "minor7")
	case "maj7", "major7", "M7":
		suffixVariants = append(suffixVariants, "maj7", "major7", "M7")
	}

	// First try direct lookup with all key and suffix variants
	var fullData string
	var err error
	for _, keyVariant := range alternateKeys {
		for _, suffixVariant := range suffixVariants {
			err = db.QueryRow(`
				SELECT full_data FROM chords 
				WHERE key = ? AND suffix = ?
			`, keyVariant, suffixVariant).Scan(&fullData)

			if err != sql.ErrNoRows {
				break
			}
		}
		if err != sql.ErrNoRows {
			break
		}
	}

	// If not found, try alias lookup with all key and suffix variants
	if err == sql.ErrNoRows {
		for _, keyVariant := range alternateKeys {
			for _, suffixVariant := range suffixVariants {
				err = db.QueryRow(`
					SELECT c.full_data 
					FROM chords c
					JOIN chord_aliases a ON c.id = a.chord_id
					WHERE a.alias_key = ? AND a.alias_suffix = ?
				`, keyVariant, suffixVariant).Scan(&fullData)

				if err != sql.ErrNoRows {
					break
				}
			}
			if err != sql.ErrNoRows {
				break
			}
		}
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
	fingering := r.URL.Path[len("/fingers/"):]
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

// searchChords handles the search endpoint that can search for both chord names and fingerings
func searchChords(w http.ResponseWriter, r *http.Request) {
	// Extract search query from URL
	query := r.URL.Path[len("/search/"):]
	if query == "" {
		http.Error(w, "Search query required", http.StatusBadRequest)
		return
	}

	// Prepare response
	w.Header().Set("Content-Type", "application/json")

	// Determine if the query is likely a fingering pattern or a chord name
	isFingeringPattern := isLikelyFingeringPattern(query)
	isChordName := isLikelyChordName(query)

	// Results to return
	var results []json.RawMessage
	var err error

	// If it's clearly a fingering pattern, search only fingerings
	if isFingeringPattern && !isChordName {
		results, err = searchByFingering(query)
	} else if isChordName && !isFingeringPattern {
		// If it's clearly a chord name, search only chord names
		results, err = searchByChordName(query)
	} else {
		// If it could be either or we're not sure, search both but prioritize simpler chords
		results, err = searchBoth(query)
	}

	if err != nil {
		http.Error(w, "Database error", http.StatusInternalServerError)
		log.Printf("Search error: %v", err)
		return
	}

	if len(results) == 0 {
		http.Error(w, "No results found", http.StatusNotFound)
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

// isLikelyFingeringPattern determines if a query is likely a fingering pattern
func isLikelyFingeringPattern(query string) bool {
	// Fingering patterns can contain:
	// - digits (0-9) for frets 0-9
	// - lowercase letters (a-z) for frets 10 and above (a=10, b=11, etc.)
	// - 'x' or 'X' for muted strings
	for _, c := range query {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'z') || c == 'x' || c == 'X') {
			return false
		}
	}
	return true
}

// isLikelyChordName determines if a query is likely a chord name
func isLikelyChordName(query string) bool {
	// Chord names typically start with a letter A-G, possibly followed by # or b
	if len(query) == 0 {
		return false
	}

	// Check if the first character is a valid chord key (A-G)
	firstChar := query[0]
	if !((firstChar >= 'A' && firstChar <= 'G') || (firstChar >= 'a' && firstChar <= 'g')) {
		return false
	}

	return true
}

// searchByFingering searches for chords by fingering pattern
func searchByFingering(query string) ([]json.RawMessage, error) {
	// Query the database for fingerings that start with the query
	rows, err := db.Query(`
		SELECT c.full_data 
		FROM chords c
		JOIN fingerings f ON c.id = f.chord_id
		WHERE f.frets LIKE ?
		LIMIT 10
	`, query+"%")

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect results
	var results []json.RawMessage
	for rows.Next() {
		var fullData string
		if err := rows.Scan(&fullData); err != nil {
			return nil, err
		}
		results = append(results, json.RawMessage(fullData))
	}

	return results, nil
}

// searchByChordName searches for chords by name
func searchByChordName(query string) ([]json.RawMessage, error) {
	// Special case for Bb/A# chords
	if strings.ToUpper(query) == "BB" || strings.HasPrefix(strings.ToUpper(query), "BB") {
		// Direct query for A# chords
		rows, err := db.Query(`
			SELECT c.full_data 
			FROM chords c
			WHERE c.key = 'A#'
			LIMIT 10
		`)

		if err != nil {
			return nil, err
		}
		defer rows.Close()

		// Collect results
		var results []json.RawMessage
		for rows.Next() {
			var fullData string
			if err := rows.Scan(&fullData); err != nil {
				return nil, err
			}
			results = append(results, json.RawMessage(fullData))
		}

		if len(results) > 0 {
			return results, nil
		}
	}

	// Special case for Am to prioritize A minor
	if strings.ToUpper(query) == "AM" || strings.ToUpper(query) == "AMIN" || strings.ToUpper(query) == "AMINOR" {
		// Direct query for A minor chord
		var fullData string
		err := db.QueryRow(`
			SELECT c.full_data 
			FROM chords c
			WHERE c.key = 'A' AND c.suffix = 'minor'
		`).Scan(&fullData)

		if err == nil {
			// Return A minor as the first result
			results := []json.RawMessage{json.RawMessage(fullData)}

			// Then get other A minor-like chords
			rows, err := db.Query(`
				SELECT c.full_data 
				FROM chords c
				WHERE c.key = 'A' AND c.suffix LIKE 'm%' AND c.suffix != 'minor'
				LIMIT 9
			`)

			if err == nil {
				defer rows.Close()

				// Add other results
				for rows.Next() {
					var data string
					if err := rows.Scan(&data); err != nil {
						continue
					}
					results = append(results, json.RawMessage(data))
				}
			}

			return results, nil
		}
	}

	// Special case for C# to prioritize C# major
	if strings.ToUpper(query) == "C#" || strings.ToUpper(query) == "C#MAJ" || strings.ToUpper(query) == "C#MAJOR" {
		// Direct query for C# major chord
		var fullData string
		err := db.QueryRow(`
			SELECT c.full_data 
			FROM chords c
			WHERE c.key = 'C#' AND c.suffix = 'major'
		`).Scan(&fullData)

		if err == nil {
			// Return C# major as the first result
			results := []json.RawMessage{json.RawMessage(fullData)}

			// Then get other C# chords
			rows, err := db.Query(`
				SELECT c.full_data 
				FROM chords c
				WHERE c.key = 'C#' AND c.suffix != 'major'
				LIMIT 9
			`)

			if err == nil {
				defer rows.Close()

				// Add other results
				for rows.Next() {
					var data string
					if err := rows.Scan(&data); err != nil {
						continue
					}
					results = append(results, json.RawMessage(data))
				}
			}

			return results, nil
		}
	}

	// Split the query into key and suffix parts
	var key, suffix string
	for i, c := range query {
		if !((c >= 'A' && c <= 'G') || (c >= 'a' && c <= 'g') || c == '#' || c == 'b') {
			key = query[:i]
			suffix = query[i:]
			break
		}
	}
	if key == "" {
		key = query
		suffix = ""
	}

	// Convert key to uppercase for consistency
	key = strings.ToUpper(key)

	// Handle common suffix aliases
	suffixAliases := []string{suffix}

	// Add common aliases based on the suffix
	switch strings.ToLower(suffix) {
	case "m", "min":
		suffixAliases = append(suffixAliases, "minor", "m", "min")
	case "":
		suffixAliases = append(suffixAliases, "major", "maj", "M", "")
	}

	// Handle enharmonic equivalents for flat/sharp notations
	alternateKeys := []string{key}

	// Map flat notations to sharp equivalents
	if len(key) == 2 && key[1] == 'b' {
		switch key[0] {
		case 'A':
			alternateKeys = append(alternateKeys, "G#")
		case 'B':
			alternateKeys = append(alternateKeys, "A#")
		case 'C':
			alternateKeys = append(alternateKeys, "B")
		case 'D':
			alternateKeys = append(alternateKeys, "C#")
		case 'E':
			alternateKeys = append(alternateKeys, "D#")
		case 'F':
			alternateKeys = append(alternateKeys, "E")
		case 'G':
			alternateKeys = append(alternateKeys, "F#")
		}
	}

	// Special case for Bb which might be capitalized differently
	if strings.ToUpper(key) == "BB" {
		alternateKeys = []string{"BB", "A#"}
		fmt.Printf("DEBUG: Special case for Bb, alternateKeys = %v\n", alternateKeys)
	}

	// Handle special enharmonic equivalents
	if key == "B#" {
		alternateKeys = append(alternateKeys, "C")
	} else if key == "E#" {
		alternateKeys = append(alternateKeys, "F")
	}

	// First try to find exact matches for common chord types
	var exactMatches []json.RawMessage

	// Define common chord types to prioritize
	commonSuffixes := []string{"", "major", "minor", "m", "7", "maj7", "m7", "dim", "aug", "sus2", "sus4"}

	// Check if the current suffix is one of the common types
	isCommonSuffix := false
	for _, s := range commonSuffixes {
		if strings.ToLower(suffix) == strings.ToLower(s) {
			isCommonSuffix = true
			break
		}
	}

	// If it's a common suffix, prioritize exact matches for these types
	if isCommonSuffix {
		for _, keyVariant := range alternateKeys {
			for _, suffixVariant := range suffixAliases {
				// Query for exact matches with common suffixes
				exactRows, err := db.Query(`
					SELECT c.full_data 
					FROM chords c
					WHERE (c.key = ? AND (c.suffix = ? OR c.suffix = ? OR c.suffix = ?))
					OR EXISTS (
						SELECT 1 FROM chord_aliases a 
						WHERE a.chord_id = c.id AND a.alias_key = ? AND (a.alias_suffix = ? OR a.alias_suffix = ? OR a.alias_suffix = ?)
					)
					ORDER BY 
						CASE 
							WHEN c.suffix = 'minor' AND ? IN ('m', 'min') THEN 0
							WHEN c.suffix = '' AND ? = '' THEN 0
							WHEN c.suffix = 'major' AND ? = '' THEN 1
							ELSE 2
						END
					LIMIT 10
				`, keyVariant, suffixVariant, "minor", "major", keyVariant, suffixVariant, "minor", "major", suffix, suffix, suffix)

				if err != nil {
					return nil, err
				}

				// Collect exact matches
				for exactRows.Next() {
					var fullData string
					if err := exactRows.Scan(&fullData); err != nil {
						exactRows.Close()
						return nil, err
					}
					exactMatches = append(exactMatches, json.RawMessage(fullData))
				}
				exactRows.Close()

				// If we found matches, return them
				if len(exactMatches) > 0 {
					return exactMatches, nil
				}
			}
		}
	}

	// If no exact matches for common types or not a common suffix, try exact matches for any suffix
	for _, keyVariant := range alternateKeys {
		for _, suffixVariant := range suffixAliases {
			// Query for exact matches
			exactRows, err := db.Query(`
				SELECT c.full_data 
				FROM chords c
				WHERE (c.key = ? AND c.suffix = ?)
				OR EXISTS (
					SELECT 1 FROM chord_aliases a 
					WHERE a.chord_id = c.id AND a.alias_key = ? AND a.alias_suffix = ?
				)
			`, keyVariant, suffixVariant, keyVariant, suffixVariant)

			if err != nil {
				return nil, err
			}

			// Collect exact matches
			for exactRows.Next() {
				var fullData string
				if err := exactRows.Scan(&fullData); err != nil {
					exactRows.Close()
					return nil, err
				}
				exactMatches = append(exactMatches, json.RawMessage(fullData))
			}
			exactRows.Close()
		}
	}

	// If we have exact matches, return them
	if len(exactMatches) > 0 {
		return exactMatches, nil
	}

	// If no exact matches, try partial matches with all key variants
	var placeholders []string
	var args []interface{}

	for _, keyVariant := range alternateKeys {
		for _, suffixVariant := range suffixAliases {
			placeholders = append(placeholders, "(c.key LIKE ? AND c.suffix LIKE ?)")
			args = append(args, keyVariant+"%", suffixVariant+"%")

			placeholders = append(placeholders, "EXISTS (SELECT 1 FROM chord_aliases a WHERE a.chord_id = c.id AND a.alias_key LIKE ? AND a.alias_suffix LIKE ?)")
			args = append(args, keyVariant+"%", suffixVariant+"%")
		}
	}

	sqlQuery := fmt.Sprintf(`
		SELECT c.full_data 
		FROM chords c
		WHERE %s
		ORDER BY 
			CASE 
				WHEN c.key = ? THEN 0 
				ELSE 1 
			END,
			CASE
				WHEN c.suffix = 'minor' AND ? IN ('m', 'min') THEN 0
				WHEN c.suffix = '' AND ? = '' THEN 0
				WHEN c.suffix = 'major' AND ? = '' THEN 1
				ELSE 2
			END,
			LENGTH(c.suffix) ASC
		LIMIT 10
	`, strings.Join(placeholders, " OR "))

	// Add parameters for the ORDER BY clause
	args = append(args, key, suffix, suffix, suffix)

	// Query the database for chord names that match any of the key variants and suffix
	rows, err := db.Query(sqlQuery, args...)

	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Collect results
	var results []json.RawMessage
	for rows.Next() {
		var fullData string
		if err := rows.Scan(&fullData); err != nil {
			return nil, err
		}
		results = append(results, json.RawMessage(fullData))
	}

	return results, nil
}

// searchBoth searches for both chord names and fingerings, prioritizing simpler chords
func searchBoth(query string) ([]json.RawMessage, error) {
	// First try chord name search
	chordResults, err := searchByChordName(query)
	if err != nil {
		return nil, err
	}

	// If we have enough chord results, return them
	if len(chordResults) >= 5 {
		return chordResults[:5], nil
	}

	// Otherwise, try fingering search as well
	fingeringResults, err := searchByFingering(query)
	if err != nil {
		return nil, err
	}

	// Combine results, prioritizing chord results
	combinedResults := append(chordResults, fingeringResults...)

	// Limit to 10 results
	if len(combinedResults) > 10 {
		combinedResults = combinedResults[:10]
	}

	return combinedResults, nil
}
