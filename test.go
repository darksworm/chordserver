package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"os/exec"
	"time"
)

// TestChordResponse represents the expected structure of a chord response
type TestChordResponse struct {
	Key       string              `json:"key"`
	Suffix    string              `json:"suffix"`
	Positions []TestChordPosition `json:"positions"`
}

// TestChordPosition represents a chord position
type TestChordPosition struct {
	Frets   string `json:"frets"`
	Fingers string `json:"fingers"`
	Barres  string `json:"barres,omitempty"`
	Capo    string `json:"capo,omitempty"`
}

func main() {
	// Define test port (different from default 8080)
	testPort := 8079

	// Test chords that were previously not found
	testChords := []string{
		"Ab", "Abmin", "B#", "B%23",
		// Add more test cases as needed
	}

	// Test fingering patterns
	testFingers := []string{
		"x47654", // A major chord with C# in bass
		"102220", // A chord with F in bass
		"x12212", // A minor 6th chord with A# in bass
		"000230", // A sus4 chord with E in bass
		"x22220", // A add9 chord with B in bass
	}

	// Test search queries
	testSearches := []struct {
		name           string
		query          string
		expectNotFound bool // true if we expect a 404 Not Found response
	}{
		{"Chord name - A", "A", false},
		{"Chord name - Am", "Am", false},
		{"Chord name - C7", "C7", false},
		{"Fingering pattern - 022000", "022000", false},
		{"Fingering pattern - 320003", "320003", false},
		{"Ambiguous - A7", "A7", false},
		// High fret fingering pattern tests
		{"High fret pattern - xmxmmm", "xmxmmm", false},
		{"High fret pattern - xxxmmm", "xxxmmm", false},
		{"High fret pattern with letters - abcdef", "abcdef", true},
		{"High fret pattern with mix - 9abcde", "9abcde", true},
	}

	// Start the server as a separate process with custom port
	cmd := exec.Command("go", "run", "server.go", "-port", fmt.Sprintf("%d", testPort))
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		fmt.Printf("ERROR: Failed to start server: %v\n", err)
		os.Exit(1)
	}

	// Ensure we kill the server when we're done
	defer cmd.Process.Kill()

	fmt.Printf("Starting server on port %d...\n", testPort)

	// Wait for the server to start and verify it's running
	if !waitForServer(testPort) {
		fmt.Printf("ERROR: Server failed to start or is not responding on port %d\n", testPort)
		os.Exit(1)
	}

	// Track test results for chords
	totalChordTests := len(testChords)
	passedChordTests := 0
	failedChordTests := 0

	// Test each chord
	fmt.Printf("\n=== TESTING CHORDS ENDPOINT ===\n\n")
	for _, chord := range testChords {
		fmt.Printf("Testing chord: %s\n", chord)

		// Make a request to the server using the test port
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/chords/%s", testPort, chord))
		if err != nil {
			fmt.Printf("ERROR: Failed to make request for %s: %v\n", chord, err)
			failedChordTests++
			continue
		}

		// Read the response
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("ERROR: Failed to read response for %s: %v\n", chord, err)
			failedChordTests++
			continue
		}

		// Check if the chord was found
		if resp.StatusCode == http.StatusOK {
			// Verify the JSON structure
			var chordData TestChordResponse
			if err := json.Unmarshal(body, &chordData); err != nil {
				fmt.Printf("ERROR: Invalid JSON response for %s: %v\n", chord, err)
				fmt.Printf("Response: %s\n", string(body))
				failedChordTests++
				continue
			}

			// Verify required fields
			if chordData.Key == "" || len(chordData.Positions) == 0 {
				fmt.Printf("ERROR: Missing required fields in response for %s\n", chord)
				fmt.Printf("Response: %s\n", string(body))
				failedChordTests++
				continue
			}

			fmt.Printf("SUCCESS: Chord %s was found with valid structure!\n", chord)
			passedChordTests++
		} else {
			fmt.Printf("FAILURE: Chord %s was not found. Status: %d\n", chord, resp.StatusCode)
			fmt.Printf("Response: %s\n", string(body))
			failedChordTests++
		}

		fmt.Println()
	}

	// Track test results for fingers
	totalFingerTests := len(testFingers)
	passedFingerTests := 0
	failedFingerTests := 0

	// Test each fingering pattern
	fmt.Printf("\n=== TESTING FINGERS ENDPOINT ===\n\n")
	for _, finger := range testFingers {
		fmt.Printf("Testing fingering pattern: %s\n", finger)

		// Make a request to the server using the test port
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/fingers/%s", testPort, finger))
		if err != nil {
			fmt.Printf("ERROR: Failed to make request for fingering %s: %v\n", finger, err)
			failedFingerTests++
			continue
		}

		// Read the response
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("ERROR: Failed to read response for fingering %s: %v\n", finger, err)
			failedFingerTests++
			continue
		}

		// Check if chords with this fingering were found
		if resp.StatusCode == http.StatusOK {
			// Verify the JSON structure - should be an array of chord objects
			var chordsData []json.RawMessage
			if err := json.Unmarshal(body, &chordsData); err != nil {
				fmt.Printf("ERROR: Invalid JSON response for fingering %s: %v\n", finger, err)
				fmt.Printf("Response: %s\n", string(body))
				failedFingerTests++
				continue
			}

			// Verify we got at least one chord
			if len(chordsData) == 0 {
				fmt.Printf("ERROR: Empty array returned for fingering %s\n", finger)
				fmt.Printf("Response: %s\n", string(body))
				failedFingerTests++
				continue
			}

			// Verify each chord has a valid structure
			validChords := true
			for i, chordJSON := range chordsData {
				var chordData TestChordResponse
				if err := json.Unmarshal(chordJSON, &chordData); err != nil {
					fmt.Printf("ERROR: Invalid chord JSON at index %d for fingering %s: %v\n", i, finger, err)
					fmt.Printf("Chord JSON: %s\n", string(chordJSON))
					validChords = false
					break
				}

				// Verify required fields
				if chordData.Key == "" || len(chordData.Positions) == 0 {
					fmt.Printf("ERROR: Missing required fields in chord at index %d for fingering %s\n", i, finger)
					fmt.Printf("Chord JSON: %s\n", string(chordJSON))
					validChords = false
					break
				}

				// Verify this chord actually has the fingering pattern we requested
				hasMatchingFingering := false
				for _, pos := range chordData.Positions {
					if pos.Frets == finger {
						hasMatchingFingering = true
						break
					}
				}

				if !hasMatchingFingering {
					fmt.Printf("ERROR: Chord at index %d does not have the requested fingering pattern %s\n", i, finger)
					fmt.Printf("Chord JSON: %s\n", string(chordJSON))
					validChords = false
					break
				}
			}

			if validChords {
				fmt.Printf("SUCCESS: Found %d chord(s) with fingering pattern %s!\n", len(chordsData), finger)
				passedFingerTests++
			} else {
				failedFingerTests++
			}
		} else {
			fmt.Printf("FAILURE: No chords found with fingering pattern %s. Status: %d\n", finger, resp.StatusCode)
			fmt.Printf("Response: %s\n", string(body))
			failedFingerTests++
		}

		fmt.Println()
	}

	// Track test results for search
	totalSearchTests := len(testSearches)
	passedSearchTests := 0
	failedSearchTests := 0

	// Test each search query
	fmt.Printf("\n=== TESTING SEARCH ENDPOINT ===\n\n")
	for _, tc := range testSearches {
		fmt.Printf("Testing %s with query '%s':\n", tc.name, tc.query)

		// Make request to search endpoint
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/search/%s", testPort, tc.query))
		if err != nil {
			fmt.Printf("ERROR: Failed to make request for search %s: %v\n", tc.query, err)
			failedSearchTests++
			continue
		}

		// Read response
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("ERROR: Failed to read response for search %s: %v\n", tc.query, err)
			failedSearchTests++
			continue
		}

		// Check status code
		if tc.expectNotFound {
			// For these patterns, we expect a 404 Not Found response
			if resp.StatusCode == http.StatusNotFound {
				fmt.Printf("SUCCESS: Search for '%s' correctly returned Not Found as expected\n", tc.query)
				passedSearchTests++
				continue
			} else {
				fmt.Printf("FAILURE: Search for '%s' should have returned Not Found but got Status: %d\n", tc.query, resp.StatusCode)
				fmt.Printf("Response: %s\n", string(body))
				failedSearchTests++
				continue
			}
		} else if resp.StatusCode != http.StatusOK {
			fmt.Printf("FAILURE: Search for '%s' failed. Status: %d\n", tc.query, resp.StatusCode)
			fmt.Printf("Response: %s\n", string(body))
			failedSearchTests++
			continue
		}

		// Parse JSON response
		var results []json.RawMessage
		if err := json.Unmarshal(body, &results); err != nil {
			fmt.Printf("ERROR: Invalid JSON response for search %s: %v\n", tc.query, err)
			fmt.Printf("Response: %s\n", string(body))
			failedSearchTests++
			continue
		}

		// Verify we got at least one result
		if len(results) == 0 {
			fmt.Printf("ERROR: No results returned for search %s\n", tc.query)
			fmt.Printf("Response: %s\n", string(body))
			failedSearchTests++
			continue
		}

		// Verify each result has a valid structure
		validResults := true
		for i, resultJSON := range results {
			var resultData map[string]interface{}
			if err := json.Unmarshal(resultJSON, &resultData); err != nil {
				fmt.Printf("ERROR: Invalid result JSON at index %d for search %s: %v\n", i, tc.query, err)
				fmt.Printf("Result JSON: %s\n", string(resultJSON))
				validResults = false
				break
			}

			// Verify required fields (key and positions)
			if resultData["key"] == nil || resultData["positions"] == nil {
				fmt.Printf("ERROR: Missing required fields in result at index %d for search %s\n", i, tc.query)
				fmt.Printf("Result JSON: %s\n", string(resultJSON))
				validResults = false
				break
			}
		}

		if validResults {
			fmt.Printf("SUCCESS: Found %d result(s) for search '%s'!\n", len(results), tc.query)
			// Print first result
			var firstResult map[string]interface{}
			if err := json.Unmarshal(results[0], &firstResult); err == nil {
				fmt.Printf("  First result: %s %s\n",
					firstResult["key"],
					firstResult["suffix"])
			}
			passedSearchTests++
		} else {
			failedSearchTests++
		}

		fmt.Println()
	}

	// Print test summary
	fmt.Printf("=== TEST SUMMARY ===\n")
	fmt.Printf("Chord tests: %d total, %d passed, %d failed\n", totalChordTests, passedChordTests, failedChordTests)
	fmt.Printf("Finger tests: %d total, %d passed, %d failed\n", totalFingerTests, passedFingerTests, failedFingerTests)
	fmt.Printf("Search tests: %d total, %d passed, %d failed\n", totalSearchTests, passedSearchTests, failedSearchTests)

	totalTests := totalChordTests + totalFingerTests + totalSearchTests
	passedTests := passedChordTests + passedFingerTests + passedSearchTests
	failedTests := failedChordTests + failedFingerTests + failedSearchTests

	fmt.Printf("Overall: %d total, %d passed, %d failed\n", totalTests, passedTests, failedTests)

	// Exit with appropriate code
	if failedTests > 0 {
		os.Exit(1)
	}
}

// waitForServer attempts to connect to the server with retries
func waitForServer(port int) bool {
	const maxRetries = 10
	const retryInterval = 500 * time.Millisecond

	for i := 0; i < maxRetries; i++ {
		// Create a context with timeout for the request
		ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
		defer cancel()

		// Create a request with the context
		req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://localhost:%d/health", port), nil)
		if err != nil {
			time.Sleep(retryInterval)
			continue
		}

		// Try to connect
		client := &http.Client{}
		resp, err := client.Do(req)

		// Even if we get a 404 (endpoint might not exist), the server is running
		if err == nil {
			resp.Body.Close()
			return true
		}

		// Try a different endpoint
		req, err = http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://localhost:%d/", port), nil)
		if err != nil {
			time.Sleep(retryInterval)
			continue
		}

		resp, err = client.Do(req)
		if err == nil {
			resp.Body.Close()
			return true
		}

		fmt.Printf("Waiting for server to start (attempt %d/%d)...\n", i+1, maxRetries)
		time.Sleep(retryInterval)
	}

	return false
}
