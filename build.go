package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
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

// ChordInfo is a simplified structure for storing in the fingering index
type ChordInfo struct {
	Key    string `json:"key"`
	Suffix string `json:"suffix"`
}

func main() {
	sourceDir := flag.String("source", "", "Source directory containing chord JSON files")
	outputDir := flag.String("output", "", "Output directory for reorganized files")
	flag.Parse()

	if *sourceDir == "" || *outputDir == "" {
		fmt.Println("Usage: go run script.go -source=/path/to/source -output=/path/to/output")
		os.Exit(1)
	}

	// Create output directories
	fingeringsDir := filepath.Join(*outputDir, "fingerings")
	namesDir := filepath.Join(*outputDir, "names")

	// Create directories
	for _, dir := range []string{fingeringsDir, namesDir} {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", dir, err)
			os.Exit(1)
		}
	}

	// Map to store the fingering data we're collecting
	fingeringMap := make(map[string][]ChordInfo)

	// Walk through the source directory
	err := filepath.Walk(*sourceDir, func(path string, info os.FileInfo, err error) error {
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

		// Extract chord info
		chordInfo := ChordInfo{
			Key:    chordData.Key,
			Suffix: chordData.Suffix,
		}

		// Process each position/fingering
		for _, pos := range chordData.Positions {
			// Add this chord to the map for this fingering
			fingeringMap[pos.Frets] = append(fingeringMap[pos.Frets], chordInfo)
		}

		// Create flat names-based files
		key := chordData.Key
		suffix := chordData.Suffix

		// Normalize and create aliases for suffix
		suffixAliases := getSuffixAliases(suffix)

		// Create a file for each alias using flat naming
		for _, alias := range suffixAliases {
			// Convert any "/" to "_" for slash-chords
			alias = strings.ReplaceAll(alias, "/", "_")

			// Create filename: e.g., "C.json", "Cmaj.json", "Dm7.json"
			var filename string
			if alias == "" {
				filename = key + ".json"
			} else {
				filename = key + alias + ".json"
			}

			outputPath := filepath.Join(namesDir, filename)

			// Write the file
			if err := ioutil.WriteFile(outputPath, data, 0644); err != nil {
				fmt.Printf("Error writing alias file %s: %v\n", outputPath, err)
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Write out the individual fingering files
	fingeringCount := 0
	for fingering, chords := range fingeringMap {
		outputPath := filepath.Join(fingeringsDir, fingering+".json")

		// Convert the data to JSON
		jsonData, err := json.Marshal(chords)
		if err != nil {
			fmt.Printf("Error creating JSON for fingering %s: %v\n", fingering, err)
			continue
		}

		// Write the file
		if err := ioutil.WriteFile(outputPath, jsonData, 0644); err != nil {
			fmt.Printf("Error writing file for fingering %s: %v\n", fingering, err)
		}
		fingeringCount++
	}

	fmt.Println("Processing complete!")
	fmt.Printf("Generated %d fingering files in %s\n", fingeringCount, fingeringsDir)
	fmt.Printf("Generated chord files by name in %s\n", namesDir)
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
