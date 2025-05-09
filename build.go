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

func main() {
	sourceDir := flag.String("source", "", "Source directory containing chord JSON files")
	outputDir := flag.String("output", "", "Output directory for reorganized files")
	skipFingeringFiles := flag.Bool("skip-fingering-files", false, "Skip generating individual fingering files to speed up build")
	flag.Parse()

	if *sourceDir == "" || *outputDir == "" {
		fmt.Println("Usage: go run script.go -source=/path/to/source -output=/path/to/output [-skip-fingering-files=false]")
		os.Exit(1)
	}

	// Create output directories
	namesDir := filepath.Join(*outputDir, "names")
	if err := os.MkdirAll(namesDir, 0755); err != nil {
		fmt.Printf("Error creating directory %s: %v\n", namesDir, err)
		os.Exit(1)
	}

	fingeringsDir := ""
	if !*skipFingeringFiles {
		fingeringsDir = filepath.Join(*outputDir, "fingerings")
		if err := os.MkdirAll(fingeringsDir, 0755); err != nil {
			fmt.Printf("Error creating directory %s: %v\n", fingeringsDir, err)
			os.Exit(1)
		}
	}

	// Track fingerings for later processing
	fingeringMap := make(map[string][]string)

	// Counters
	chordNameFilesCount := 0

	// Process all files first for chord names
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

		// Track fingerings if not skipped
		if !*skipFingeringFiles {
			chordID := fmt.Sprintf("%s%s", chordData.Key, chordData.Suffix)
			for _, pos := range chordData.Positions {
				fingeringMap[pos.Frets] = append(fingeringMap[pos.Frets], chordID)
			}
		}

		// Create minified version of the data for storage
		minifiedData, err := json.Marshal(chordData)
		if err != nil {
			fmt.Printf("Error minifying JSON from %s: %v\n", path, err)
			return nil
		}

		// Create flat names-based files
		key := chordData.Key
		suffix := chordData.Suffix

		// Normalize and create aliases for suffix
		suffixAliases := getSuffixAliases(suffix)

		// Create a file for each alias using flat naming
		for _, alias := range suffixAliases {
			var filename string
			var targetDir string

			// Handle slash chords properly
			if strings.Contains(alias, "/") {
				// For slash chords like "G/B", split into directory and filename
				parts := strings.Split(alias, "/")
				if len(parts) == 2 {
					// If it's a simple slash chord
					if parts[0] == "" {
						filename = key + ".json"
					} else {
						filename = key + parts[0] + ".json"
					}

					// Create subdirectory for the bass note
					targetDir = filepath.Join(namesDir, parts[1])
					if err := os.MkdirAll(targetDir, 0755); err != nil {
						fmt.Printf("Error creating directory %s: %v\n", targetDir, err)
						continue
					}
				} else {
					// More complex slash chord, just use URL encoding
					encodedAlias := strings.ReplaceAll(alias, "/", "%2F")
					if encodedAlias == "" {
						filename = key + ".json"
					} else {
						filename = key + encodedAlias + ".json"
					}
					targetDir = namesDir
				}
			} else {
				// Regular chord (no slash)
				if alias == "" {
					filename = key + ".json"
				} else {
					filename = key + alias + ".json"
				}
				targetDir = namesDir
			}

			outputPath := filepath.Join(targetDir, filename)

			// Write the file
			if err := ioutil.WriteFile(outputPath, minifiedData, 0644); err != nil {
				fmt.Printf("Error writing alias file %s: %v\n", outputPath, err)
			} else {
				chordNameFilesCount++
			}
		}

		return nil
	})

	if err != nil {
		fmt.Printf("Error walking directory: %v\n", err)
		os.Exit(1)
	}

	// Generate fingering files afterward if needed
	fingeringCount := 0
	if !*skipFingeringFiles {
		for fingering, chords := range fingeringMap {
			outputPath := filepath.Join(fingeringsDir, fingering+".json")

			// Convert the data to minified JSON
			jsonData, err := json.Marshal(chords)
			if err != nil {
				fmt.Printf("Error creating JSON for fingering %s: %v\n", fingering, err)
				continue
			}

			// Write the file
			if err := ioutil.WriteFile(outputPath, jsonData, 0644); err != nil {
				fmt.Printf("Error writing file for fingering %s: %v\n", fingering, err)
			} else {
				fingeringCount++
			}
		}
	}

	fmt.Println("Processing complete!")
	fmt.Printf("Generated %d chord name files in %s\n", chordNameFilesCount, namesDir)
	if !*skipFingeringFiles {
		fmt.Printf("Generated %d fingering files in %s\n", fingeringCount, fingeringsDir)
	} else {
		fmt.Println("Skipped generation of individual fingering files")
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
