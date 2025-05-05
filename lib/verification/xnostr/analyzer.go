package xnostr

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
)

// analyzeScreenshot analyzes a screenshot with Llama 3.2 Vision
func (s *Service) analyzeScreenshot(imagePath string) (ProfileData, error) {
	// Use multiple retries for more consistent results
	return s.analyzeWithRetries(imagePath, 3)
}

// analyzeWithRetries attempts to analyze the image multiple times for more consistent results
func (s *Service) analyzeWithRetries(imagePath string, retries int) (ProfileData, error) {
	var results []ProfileData
	var errors []error

	// Perform multiple analyses
	for i := 0; i < retries; i++ {
		fmt.Printf("Analysis attempt %d/%d...\n", i+1, retries)

		// Analyze the image with Llama Vision
		llamaResponse, err := s.analyzeImageWithLlama(imagePath)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		// Extract profile data
		profileData, err := s.extractProfileData(llamaResponse)
		if err != nil {
			errors = append(errors, err)
			continue
		}

		results = append(results, profileData)
	}

	// If all attempts failed, return the last error
	if len(results) == 0 {
		if len(errors) > 0 {
			return ProfileData{}, fmt.Errorf("all analysis attempts failed: %v", errors[len(errors)-1])
		}
		return ProfileData{}, fmt.Errorf("all analysis attempts failed with unknown errors")
	}

	// If we only have one result, return it
	if len(results) == 1 {
		return results[0], nil
	}

	// Implement a simple voting mechanism for more consistent results
	// Count occurrences of each follower count (ignoring empty values)
	followerCounts := make(map[string]int)
	for _, result := range results {
		if result.FollowerCount != "" {
			followerCounts[result.FollowerCount]++
		}
	}

	// Find the most common follower count
	var mostCommonCount string
	var maxOccurrences int
	for count, occurrences := range followerCounts {
		if occurrences > maxOccurrences {
			mostCommonCount = count
			maxOccurrences = occurrences
		}
	}

	// Count occurrences of each npub (ignoring empty values)
	npubs := make(map[string]int)
	for _, result := range results {
		if result.Npub != "" {
			npubs[result.Npub]++
		}
	}

	// Find the most common npub
	var mostCommonNpub string
	maxOccurrences = 0
	for npub, occurrences := range npubs {
		if occurrences > maxOccurrences {
			mostCommonNpub = npub
			maxOccurrences = occurrences
		}
	}

	// Debug: Print all results for debugging
	fmt.Println("All analysis results:")
	for i, result := range results {
		fmt.Printf("Result %d: npub=%v, follower_count=%v\n", i+1, result.Npub, result.FollowerCount)
	}
	fmt.Printf("Most common follower count: %v (occurrences: %d)\n", mostCommonCount, maxOccurrences)
	fmt.Printf("Most common npub: %v\n", mostCommonNpub)

	// Return the most consistent result
	profileData := ProfileData{
		Npub:          mostCommonNpub,
		FollowerCount: mostCommonCount,
	}

	// Ensure we're not returning an empty result if we have data
	if profileData.Npub == "" && profileData.FollowerCount == "" && len(results) > 0 {
		// If voting mechanism failed, use the first non-empty result
		for _, result := range results {
			if result.FollowerCount != "" {
				profileData.FollowerCount = result.FollowerCount
			}
			if result.Npub != "" {
				profileData.Npub = result.Npub
			}
		}
	}

	return profileData, nil
}

// analyzeImageWithLlama sends the image to Ollama's Llama 3.2 Vision model for analysis
func (s *Service) analyzeImageWithLlama(imagePath string) (string, error) {
	// Read the image file
	imageData, err := os.ReadFile(imagePath)
	if err != nil {
		return "", fmt.Errorf("failed to read image file: %w", err)
	}

	// Encode the image to base64
	base64Image := base64.StdEncoding.EncodeToString(imageData)

	// Prepare the request to Ollama
	ollamaReq := OllamaRequest{
		Model:  "llama3.2-vision",
		Prompt: "Analyze this Twitter/X profile screenshot. I need you to extract ONLY two pieces of information:\n\n1. The npub (Nostr public key) if present - this would look like 'npub1...' followed by alphanumeric characters\n2. The EXACT follower count as displayed on the profile\n\nIMPORTANT INSTRUCTIONS FOR NPUB:\n- The npub may be found in the bio text section of the profile\n- It would look like 'npub1' followed by alphanumeric characters\n- Check the entire bio text carefully for any mention of npub\n- The npub might be on its own line in the bio, separate from other text\n- Look for any text that starts with 'npub1' followed by a long string of letters and numbers\n\nIMPORTANT INSTRUCTIONS FOR FOLLOWER COUNT:\n- On Twitter/X or Nitter, the profile stats are typically displayed in a row with several numbers\n- The stats row usually shows: Tweets (or Posts), Following, Followers, Likes\n- The follower count is the number that appears directly under or next to the word 'Followers'\n- On Nitter specifically, the stats are shown as a row of numbers with labels underneath\n- Be extremely careful not to confuse the follower count with other numbers like Tweet count or Following count\n- In the Nitter interface, the follower count is typically the THIRD number in the stats row\n- Report the EXACT number as shown, preserving any abbreviations (K, M, etc.)\n\nReturn ONLY a JSON object with keys 'npub' and 'follower_count'. If either is not found, set the value to null.",
		Stream: false,
		Images: []string{base64Image},
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request to Ollama API
	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return "", fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %w", err)
	}

	// Extract the response text
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return "", fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Get the response text
	responseText, ok := result["response"].(string)
	if !ok {
		return "", fmt.Errorf("invalid response format from Ollama")
	}

	return responseText, nil
}
