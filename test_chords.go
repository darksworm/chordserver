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

	// Track test results
	totalTests := len(testChords)
	passedTests := 0
	failedTests := 0

	// Test each chord
	for _, chord := range testChords {
		fmt.Printf("Testing chord: %s\n", chord)

		// Make a request to the server using the test port
		resp, err := http.Get(fmt.Sprintf("http://localhost:%d/chords/%s", testPort, chord))
		if err != nil {
			fmt.Printf("ERROR: Failed to make request for %s: %v\n", chord, err)
			failedTests++
			continue
		}

		// Read the response
		body, err := ioutil.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			fmt.Printf("ERROR: Failed to read response for %s: %v\n", chord, err)
			failedTests++
			continue
		}

		// Check if the chord was found
		if resp.StatusCode == http.StatusOK {
			// Verify the JSON structure
			var chordData TestChordResponse
			if err := json.Unmarshal(body, &chordData); err != nil {
				fmt.Printf("ERROR: Invalid JSON response for %s: %v\n", chord, err)
				fmt.Printf("Response: %s\n", string(body))
				failedTests++
				continue
			}

			// Verify required fields
			if chordData.Key == "" || len(chordData.Positions) == 0 {
				fmt.Printf("ERROR: Missing required fields in response for %s\n", chord)
				fmt.Printf("Response: %s\n", string(body))
				failedTests++
				continue
			}

			fmt.Printf("SUCCESS: Chord %s was found with valid structure!\n", chord)
			passedTests++
		} else {
			fmt.Printf("FAILURE: Chord %s was not found. Status: %d\n", chord, resp.StatusCode)
			fmt.Printf("Response: %s\n", string(body))
			failedTests++
		}

		fmt.Println()
	}

	// Print test summary
	fmt.Printf("=== TEST SUMMARY ===\n")
	fmt.Printf("Total tests: %d\n", totalTests)
	fmt.Printf("Passed: %d\n", passedTests)
	fmt.Printf("Failed: %d\n", failedTests)

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
