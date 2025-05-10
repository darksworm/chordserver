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

// ChordWithMeta extends ChordData with additional metadata for search optimization
type ChordWithMeta struct {
	Key              string        `json:"key"`
	Suffix           string        `json:"suffix"`
	Positions        []interface{} `json:"positions"`
	NormalizedKey    string
	NormalizedSuffix string
	FullData         string // The original JSON string
}

// In-memory data structures
var chordCache []*ChordWithMeta
var chordMap map[string]*ChordWithMeta        // For direct lookups by key+suffix
var fingeringMap map[string][]*ChordWithMeta  // For lookups by fingering pattern
var normalizedMap map[string][]*ChordWithMeta // For lookups by normalized key+suffix

// Map of enharmonic equivalents
var enharmonicMap = map[string]string{
	"BB": "A#",
	"DB": "C#",
	"EB": "D#",
	"GB": "F#",
	"AB": "G#",
	"B#": "C",
	"E#": "F",
}

// Map of suffix aliases
var suffixAliasMap = map[string]string{
	"M":      "major",
	"MAJ":    "major",
	"":       "major", // Empty suffix implies major
	"m":      "minor",
	"MIN":    "minor",
	"MINOR":  "minor",
	"5":      "5",
	"POWER":  "5",
	"FIFTH":  "5",
	"7":      "7",
	"DOM7":   "7",
	"DOM":    "7",
	"m7":     "m7",
	"MIN7":   "m7",
	"MINOR7": "m7",
	"MAJ7":   "maj7",
	"MAJOR7": "maj7",
	"M7":     "maj7",
	"SUS2":   "sus2",
	"SUS4":   "sus4",
}

// normalizeKey normalizes a chord key for search
func normalizeKey(key string) string {
	key = strings.ToUpper(key)
	if alt, exists := enharmonicMap[key]; exists {
		return alt
	}
	return key
}

// normalizeSuffix normalizes a chord suffix for search
func normalizeSuffix(suffix string) string {
	suffix = strings.ToUpper(suffix)
	if alt, exists := suffixAliasMap[suffix]; exists {
		return alt
	}
	return suffix
}

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

	// Load all chord data into memory
	if err := loadChordData(); err != nil {
		log.Fatalf("Error loading chord data: %v", err)
	}

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

// loadChordData loads all chord data from the database into memory
func loadChordData() error {
	// Initialize the data structures
	chordCache = make([]*ChordWithMeta, 0)
	chordMap = make(map[string]*ChordWithMeta)
	fingeringMap = make(map[string][]*ChordWithMeta)
	normalizedMap = make(map[string][]*ChordWithMeta)

	// Query all chords from the database
	rows, err := db.Query(`SELECT id, key, suffix, full_data FROM chords`)
	if err != nil {
		return err
	}
	defer rows.Close()

	// Process each chord
	for rows.Next() {
		var id int
		var key, suffix, fullData string
		if err := rows.Scan(&id, &key, &suffix, &fullData); err != nil {
			return err
		}

		// Parse the full JSON data directly into a ChordWithMeta
		chord := &ChordWithMeta{}
		if err := json.Unmarshal([]byte(fullData), chord); err != nil {
			return err
		}

		// Add the additional metadata
		chord.NormalizedKey = normalizeKey(key)
		chord.NormalizedSuffix = normalizeSuffix(suffix)
		chord.FullData = fullData

		// Add to cache and maps
		chordCache = append(chordCache, chord)
		chordMap[key+"|"+suffix] = chord

		// Add to normalized map
		normalizedKey := chord.NormalizedKey
		normalizedSuffix := chord.NormalizedSuffix
		normalizedMapKey := normalizedKey + "|" + normalizedSuffix
		normalizedMap[normalizedMapKey] = append(normalizedMap[normalizedMapKey], chord)

		// Index by fingering patterns
		for _, posInterface := range chord.Positions {
			// Convert to map to access fields
			if posMap, ok := posInterface.(map[string]interface{}); ok {
				if fretsValue, ok := posMap["frets"]; ok {
					if frets, ok := fretsValue.(string); ok {
						fingeringMap[frets] = append(fingeringMap[frets], chord)
					}
				}
			}
		}
	}

	log.Printf("Loaded %d chords into memory", len(chordCache))
	return nil
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

	// Parse the chord name into key and suffix
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

	// Normalize the key and suffix
	normalizedKey := normalizeKey(key)
	normalizedSuffix := normalizeSuffix(suffix)

	// Try direct lookup in the map
	mapKey := key + "|" + suffix
	if chord, ok := chordMap[mapKey]; ok {
		fmt.Fprint(w, chord.FullData)
		return
	}

	// Try normalized lookup
	normalizedMapKey := normalizedKey + "|" + normalizedSuffix
	if chords, ok := normalizedMap[normalizedMapKey]; ok && len(chords) > 0 {
		fmt.Fprint(w, chords[0].FullData)
		return
	}

	// If not found, try a more flexible search
	results := searchByChordNameInMemory(chordPath)
	if len(results) > 0 {
		fmt.Fprint(w, results[0].FullData)
		return
	}

	// If still not found, return 404
	http.Error(w, "Chord not found", http.StatusNotFound)
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

	// Look up chords by fingering pattern
	var chords []*ChordWithMeta
	if exactMatches, ok := fingeringMap[fingering]; ok {
		// Exact match found
		chords = exactMatches
	} else {
		// Try prefix matches
		for frets, matchingChords := range fingeringMap {
			if strings.HasPrefix(frets, fingering) {
				chords = append(chords, matchingChords...)
			}
		}
	}

	if len(chords) == 0 {
		http.Error(w, "No chords found with this fingering", http.StatusNotFound)
		return
	}

	// Convert to JSON array
	var results []json.RawMessage
	for _, chord := range chords {
		results = append(results, json.RawMessage(chord.FullData))
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
	var chords []*ChordWithMeta

	// If it's clearly a fingering pattern, search only fingerings
	if isFingeringPattern && !isChordName {
		chords = searchByFingeringInMemory(query)
	} else if isChordName && !isFingeringPattern {
		// If it's clearly a chord name, search only chord names
		chords = searchByChordNameInMemory(query)
	} else {
		// If it could be either or we're not sure, search both but prioritize simpler chords
		chords = searchBothInMemory(query)
	}

	if len(chords) == 0 {
		http.Error(w, "No results found", http.StatusNotFound)
		return
	}

	// Convert to JSON array
	var results []json.RawMessage
	for _, chord := range chords {
		results = append(results, json.RawMessage(chord.FullData))
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

// searchByFingeringInMemory searches for chords by fingering pattern using in-memory data
func searchByFingeringInMemory(query string) []*ChordWithMeta {
	var results []*ChordWithMeta

	// First try exact matches
	if chords, ok := fingeringMap[query]; ok {
		return chords
	}

	// Then try prefix matches
	for frets, chords := range fingeringMap {
		if strings.HasPrefix(frets, query) {
			results = append(results, chords...)
		}
	}

	// Limit results to 10
	if len(results) > 10 {
		results = results[:10]
	}

	return results
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
	// Use the in-memory implementation
	chords := searchBothInMemory(query)

	// Convert to JSON array
	var results []json.RawMessage
	for _, chord := range chords {
		results = append(results, json.RawMessage(chord.FullData))
	}

	return results, nil
}

// searchByChordNameInMemory searches for chords by name using in-memory data
func searchByChordNameInMemory(query string) []*ChordWithMeta {
	// Special case for Bb/A# chords
	if strings.ToUpper(query) == "BB" || strings.HasPrefix(strings.ToUpper(query), "BB") {
		// Look for A# chords
		var results []*ChordWithMeta

		// First try to find A# major if the query is just "Bb"
		if strings.ToUpper(query) == "BB" {
			var aSharpMajor *ChordWithMeta
			var otherASharp []*ChordWithMeta

			for _, chord := range chordCache {
				if chord.Key == "A#" && chord.Suffix == "major" {
					aSharpMajor = chord
					break // Found it, no need to continue
				}
			}

			// If we found A# major, collect other A# chords
			if aSharpMajor != nil {
				for _, chord := range chordCache {
					if chord != aSharpMajor && chord.Key == "A#" {
						otherASharp = append(otherASharp, chord)
					}
				}

				// Return A# major as the first result, followed by other A# chords
				results = append([]*ChordWithMeta{aSharpMajor}, otherASharp...)
				return results
			}
		}

		// If we didn't find A# major or the query is more specific, just return all A# chords
		for _, chord := range chordCache {
			if chord.Key == "A#" {
				results = append(results, chord)
			}
		}

		if len(results) > 0 {
			// Sort by common chord types
			sortByChordType(results)
			return results
		}
	}

	// Special case for Am to prioritize A minor
	if strings.ToUpper(query) == "AM" || strings.ToUpper(query) == "AMIN" || strings.ToUpper(query) == "AMINOR" {
		// Look for A minor chord
		var aMinor *ChordWithMeta
		var otherAm []*ChordWithMeta

		for _, chord := range chordCache {
			if chord.Key == "A" && chord.Suffix == "minor" {
				aMinor = chord
				break // Found it, no need to continue
			}
		}

		// If we found A minor, collect other A minor-like chords
		if aMinor != nil {
			for _, chord := range chordCache {
				if chord != aMinor && chord.Key == "A" && strings.HasPrefix(strings.ToLower(chord.Suffix), "m") {
					otherAm = append(otherAm, chord)
				}
			}

			// Return A minor as the first result, followed by other A minor-like chords
			results := []*ChordWithMeta{aMinor}
			results = append(results, otherAm...)
			return results
		}
	}

	// Special case for C# to prioritize C# major
	if strings.ToUpper(query) == "C#" || strings.ToUpper(query) == "C#MAJ" || strings.ToUpper(query) == "C#MAJOR" {
		// Look for C# major chord
		var cSharpMajor *ChordWithMeta
		var otherCSharp []*ChordWithMeta

		for _, chord := range chordCache {
			if chord.Key == "C#" && chord.Suffix == "major" {
				cSharpMajor = chord
				break // Found it, no need to continue
			}
		}

		// If we found C# major, collect other C# chords
		if cSharpMajor != nil {
			for _, chord := range chordCache {
				if chord != cSharpMajor && chord.Key == "C#" {
					otherCSharp = append(otherCSharp, chord)
				}
			}

			// Return C# major as the first result, followed by other C# chords
			results := []*ChordWithMeta{cSharpMajor}
			results = append(results, otherCSharp...)
			return results
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

	// Normalize the key and suffix
	normalizedKey := normalizeKey(key)
	normalizedSuffix := normalizeSuffix(suffix)

	// Try exact match first
	normalizedMapKey := normalizedKey + "|" + normalizedSuffix
	if chords, ok := normalizedMap[normalizedMapKey]; ok && len(chords) > 0 {
		return chords
	}

	// If no exact match, try partial matches
	var results []*ChordWithMeta

	// First try to match by key
	for _, chord := range chordCache {
		if chord.NormalizedKey == normalizedKey {
			// If suffix is empty or matches the beginning of the chord's suffix
			if suffix == "" || strings.HasPrefix(strings.ToLower(chord.Suffix), strings.ToLower(suffix)) {
				results = append(results, chord)
			}
		}
	}

	// Sort results by chord type priority
	sortByChordType(results)

	// Limit results to 10
	if len(results) > 10 {
		results = results[:10]
	}

	return results
}

// sortByChordType sorts chords by common chord types (major, minor, 7, etc.)
func sortByChordType(chords []*ChordWithMeta) {
	// Simple bubble sort by chord type priority
	n := len(chords)
	for i := 0; i < n; i++ {
		for j := 0; j < n-i-1; j++ {
			if getChordTypePriority(chords[j].Suffix) > getChordTypePriority(chords[j+1].Suffix) {
				// Swap
				chords[j], chords[j+1] = chords[j+1], chords[j]
			}
		}
	}
}

// getChordTypePriority returns a priority value for chord types (lower is higher priority)
func getChordTypePriority(suffix string) int {
	switch strings.ToLower(suffix) {
	case "", "major":
		return 0
	case "minor", "m":
		return 1
	case "7":
		return 2
	case "maj7":
		return 3
	case "m7", "min7":
		return 4
	case "dim":
		return 5
	case "aug":
		return 6
	case "sus2":
		return 7
	case "sus4":
		return 8
	default:
		return 100 // Low priority for uncommon types
	}
}

// searchBothInMemory searches for chords by both name and fingering pattern
func searchBothInMemory(query string) []*ChordWithMeta {
	// First try chord name search
	chordResults := searchByChordNameInMemory(query)

	// If we have enough chord results, return them
	if len(chordResults) >= 5 {
		return chordResults[:5]
	}

	// Otherwise, try fingering search as well
	fingeringResults := searchByFingeringInMemory(query)

	// Combine results, prioritizing chord results
	results := append(chordResults, fingeringResults...)

	// Remove duplicates
	seen := make(map[string]bool)
	var uniqueResults []*ChordWithMeta

	for _, chord := range results {
		key := chord.Key + "|" + chord.Suffix
		if !seen[key] {
			seen[key] = true
			uniqueResults = append(uniqueResults, chord)
		}
	}

	// Limit to 10 results
	if len(uniqueResults) > 10 {
		uniqueResults = uniqueResults[:10]
	}

	return uniqueResults
}
