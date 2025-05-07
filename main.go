package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"path/filepath"
	"strings"
)

const baseDir = "./json"

func main() {
	http.HandleFunc("/chords/", chordHandler)
	http.HandleFunc("/chords", chordHandler) // support ?name=
	fmt.Println("Listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}

func chordHandler(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers for wide-open access
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "*")

	// Handle preflight OPTIONS requests
	if r.Method == "OPTIONS" {
		w.WriteHeader(http.StatusOK)
		return
	}

	// 1) get chord name from /chords/{name} or ?name=
	chord := ""
	if q := r.URL.Query().Get("name"); q != "" {
		chord = q
	} else {
		chord = strings.TrimPrefix(r.URL.Path, "/chords/")
	}
	chord = strings.TrimSpace(chord)
	if chord == "" {
		http.Error(w, "missing chord name", http.StatusBadRequest)
		return
	}

	// 2) resolve to a JSON file
	filePath, err := resolveChordFile(chord)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// 3) read & serve
	data, err := ioutil.ReadFile(filePath)
	if err != nil {
		http.Error(w, "chord not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Write(data)
}

// resolveChordFile maps an input like "Am13/G" → "./json/A/m13_g.json"
func resolveChordFile(chord string) (string, error) {
	chord = strings.TrimSpace(chord)
	if chord == "" {
		return "", fmt.Errorf("empty chord")
	}

	// Extract root (1 or 2 chars if sharp/flat)
	root := ""
	rest := ""
	if len(chord) >= 2 && (chord[1] == '#' || chord[1] == 'b') {
		root = chord[:2]
		rest = chord[2:]
	} else {
		root = chord[:1]
		rest = chord[1:]
	}
	// Normalize root: letter uppercase, keep '#' or 'b'
	root = strings.ToUpper(string(root[0])) + root[1:]

	// Normalize type/suffix
	t := strings.ToLower(strings.TrimSpace(rest))
	var fileBase string
	switch t {
	case "", "maj", "major":
		fileBase = "major"
	case "m", "min", "minor":
		fileBase = "minor"
	default:
		// convert any "/" to "_" for slash‐chords
		fileBase = strings.ReplaceAll(t, "/", "_")
	}

	// Build path
	p := filepath.Join(baseDir, root, fileBase+".json")
	return p, nil
}
