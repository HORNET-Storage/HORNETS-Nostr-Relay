package xnostr

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-rod/rod"
)

// captureProfileHTML captures the HTML of the profile card
func (s *Service) captureProfileHTML(page *rod.Page) string {
	// Get raw HTML for the profile card
	cardHTML, err := page.Eval(`() => {
		const profileCard = document.querySelector('.profile-card');
		return profileCard ? profileCard.outerHTML : 'Profile card not found';
	}`)

	if err != nil {
		fmt.Printf("⚠️ Failed to capture profile card HTML: %v\n", err)
		return ""
	}

	// Convert the result to a string
	if cardHTML.Type == "string" {
		return fmt.Sprintf("%v", cardHTML.Value)
	}

	return ""
}

// captureStatsHTML captures the HTML of the stats section
func (s *Service) captureStatsHTML(page *rod.Page) string {
	// Get raw HTML for the stats section
	statsHTML, err := page.Eval(`() => {
		const statsList = document.querySelector('.profile-statlist');
		if (!statsList) return "No stats list found";
		return statsList.outerHTML;
	}`)

	if err != nil {
		fmt.Printf("⚠️ Failed to capture stats HTML: %v\n", err)
		return ""
	}

	// Convert the result to a string
	if statsHTML.Type == "string" {
		return fmt.Sprintf("%v", statsHTML.Value)
	}

	return ""
}

// extractProfileDataFromHTML uses an LLM to extract profile data from HTML
func (s *Service) extractProfileDataFromHTML(profileHTML, statsHTML string) (ProfileData, error) {
	// Combine the HTML for analysis
	combinedHTML := fmt.Sprintf("Profile Card HTML:\n%s\n\nStats HTML:\n%s", profileHTML, statsHTML)

	// Print the combined HTML for debugging
	fmt.Println("=== HTML SENT TO LLM ===")
	fmt.Println(combinedHTML)
	fmt.Println("=== END HTML ===")

	// Prepare the prompt for the LLM
	prompt := `You are a data extraction specialist. Extract the following information from the provided Twitter/X profile HTML:

1. Username (with @ symbol) - This should be the full Twitter handle like "@username", not just the display name
2. Full name
3. Bio text
4. Location (if present)
5. Website URL (if present)
6. Join date
7. Tweet count
8. Following count
9. Follower count
10. Likes count
11. Npub (Nostr public key) if present - this would look like 'npub1...' followed by alphanumeric characters

IMPORTANT: Pay special attention to the bio text, as it may contain an npub (Nostr public key). The npub would look like 'npub1' followed by alphanumeric characters. The npub might be on its own line in the bio, separate from other text. Carefully examine the entire bio content, including any text that appears to be on separate lines. If you find an npub in the bio, extract it and include it in the 'npub' field of your response.

The HTML is from a Nitter instance (a Twitter/X frontend). Pay special attention to the follower count, which is typically in the stats section.

Return the data in this JSON format:
{
  "username": "string or null",
  "full_name": "string or null",
  "bio": "string or null",
  "location": "string or null",
  "website": "string or null",
  "join_date": "string or null",
  "tweet_count": "string or null",
  "following_count": "string or null",
  "follower_count": "string or null",
  "likes_count": "string or null",
  "npub": "string or null"
}

If any field is not found, set it to null. For numeric values, return them as strings to preserve formatting (e.g., "1,234" not 1234).`

	// Prepare the request to Ollama
	ollamaReq := OllamaRequest{
		Model:  "gemma3:1b", // Use Gemma 3 1B model - smaller but faster
		Prompt: prompt + "\n\nHTML to analyze:\n" + combinedHTML,
		Stream: false,
	}

	reqBody, err := json.Marshal(ollamaReq)
	if err != nil {
		return ProfileData{}, fmt.Errorf("failed to marshal request: %w", err)
	}

	// Send request to Ollama API
	resp, err := http.Post("http://localhost:11434/api/generate", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		return ProfileData{}, fmt.Errorf("failed to send request to Ollama: %w", err)
	}
	defer resp.Body.Close()

	// Read the response
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return ProfileData{}, fmt.Errorf("failed to read response: %w", err)
	}

	// Extract the response text
	var result map[string]interface{}
	if err := json.Unmarshal(respBody, &result); err != nil {
		return ProfileData{}, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	// Get the response text
	responseText, ok := result["response"].(string)
	if !ok {
		return ProfileData{}, fmt.Errorf("invalid response format from Ollama")
	}

	fmt.Println("LLM Response:")
	fmt.Println(responseText)

	// Extract the JSON from the response
	return s.extractProfileData(responseText)
}

// extractProfileDataFromDOM attempts to extract profile data directly from the page DOM
func (s *Service) extractProfileDataFromDOM(page *rod.Page) (ProfileData, error) {
	var profileData ProfileData

	// Try to extract follower count
	followerSelectors := []string{
		"a[href$='/followers'] span",                   // Twitter follower count
		"[data-testid='followersCount']",               // Another possible Twitter selector
		".profile-stat:nth-child(3) .profile-stat-num", // Nitter follower count (3rd stat)
	}

	// Try each selector
	for _, selector := range followerSelectors {
		element, err := page.Timeout(2 * time.Second).Element(selector)
		if err == nil && element != nil {
			text, err := element.Text()
			if err == nil && text != "" {
				profileData.FollowerCount = text
				break
			}
		}
	}

	// Try to extract npub from the page
	npubSelectors := []string{
		"[data-testid='UserDescription'] a[href*='npub']", // Twitter bio with npub link
		".profile-bio a[href*='npub']",                    // Nitter bio with npub link
	}

	for _, selector := range npubSelectors {
		element, err := page.Timeout(2 * time.Second).Element(selector)
		if err == nil && element != nil {
			text, err := element.Text()
			if err == nil && strings.Contains(text, "npub") {
				profileData.Npub = text
				break
			}

			// If the text doesn't contain npub, check the href attribute
			href, err := element.Attribute("href")
			if err == nil && href != nil && strings.Contains(*href, "npub") {
				// Extract npub from href
				npubStart := strings.Index(*href, "npub")
				if npubStart >= 0 {
					npubEnd := npubStart
					for npubEnd < len(*href) && ((*href)[npubEnd] >= 'a' && (*href)[npubEnd] <= 'z' || (*href)[npubEnd] >= '0' && (*href)[npubEnd] <= '9') {
						npubEnd++
					}
					if npubEnd > npubStart {
						profileData.Npub = (*href)[npubStart:npubEnd]
					}
				}
				break
			}
		}
	}

	// If we couldn't find npub in links, try to find it directly in the bio text
	if profileData.Npub == "" {
		// Get the bio text
		bioSelectors := []string{
			".profile-bio",                    // Nitter bio
			"[data-testid='UserDescription']", // Twitter bio
		}

		for _, selector := range bioSelectors {
			element, err := page.Timeout(2 * time.Second).Element(selector)
			if err == nil && element != nil {
				bioText, err := element.Text()
				if err == nil && bioText != "" {
					// Look for npub pattern in the bio text
					npubIndex := strings.Index(bioText, "npub1")
					if npubIndex >= 0 {
						// Extract the npub
						npubEnd := npubIndex
						for npubEnd < len(bioText) && npubEnd < npubIndex+64 &&
							((bioText[npubEnd] >= 'a' && bioText[npubEnd] <= 'z') ||
								(bioText[npubEnd] >= '0' && bioText[npubEnd] <= '9')) {
							npubEnd++
						}
						if npubEnd > npubIndex {
							profileData.Npub = bioText[npubIndex:npubEnd]
							break
						}
					}
				}
			}
		}
	}

	// If we couldn't find any data, return an error
	if profileData.FollowerCount == "" && profileData.Npub == "" {
		return profileData, fmt.Errorf("could not extract profile data from DOM")
	}

	return profileData, nil
}

// extractProfileData parses the model response to extract profile data
func (s *Service) extractProfileData(modelResponse string) (ProfileData, error) {
	var profileData ProfileData

	// Extract JSON from markdown code blocks if present
	jsonStr := modelResponse
	if strings.Contains(modelResponse, "```") {
		// Find the JSON code block
		startIndex := strings.Index(modelResponse, "```")
		if startIndex >= 0 {
			// Skip the opening ```
			startIndex += 3
			// Skip "json" if present after the opening ```
			if len(modelResponse) > startIndex+4 && modelResponse[startIndex:startIndex+4] == "json" {
				startIndex += 4
			}

			// Find the closing ```
			endIndex := strings.Index(modelResponse[startIndex:], "```")
			if endIndex >= 0 {
				jsonStr = modelResponse[startIndex : startIndex+endIndex]
			}
		}
	} else {
		// If no code block, try to find JSON object directly
		jsonStart := strings.Index(modelResponse, "{")
		jsonEnd := strings.LastIndex(modelResponse, "}")
		if jsonStart >= 0 && jsonEnd > jsonStart {
			jsonStr = modelResponse[jsonStart : jsonEnd+1]
		}
	}

	// Parse the JSON
	var data map[string]interface{}
	if err := json.Unmarshal([]byte(jsonStr), &data); err != nil {
		return profileData, fmt.Errorf("failed to parse JSON: %w", err)
	}

	// Extract all fields from the JSON
	// Core fields
	if npub, ok := data["npub"]; ok && npub != nil {
		if npubStr, ok := npub.(string); ok {
			profileData.Npub = npubStr
		}
	}

	if followerCount, ok := data["follower_count"]; ok && followerCount != nil {
		if followerCountStr, ok := followerCount.(string); ok {
			profileData.FollowerCount = followerCountStr
		} else if followerCountFloat, ok := followerCount.(float64); ok {
			profileData.FollowerCount = fmt.Sprintf("%g", followerCountFloat)
		}
	}

	// Additional fields
	if username, ok := data["username"]; ok && username != nil {
		if usernameStr, ok := username.(string); ok {
			profileData.Username = usernameStr
		}
	}

	if fullName, ok := data["full_name"]; ok && fullName != nil {
		if fullNameStr, ok := fullName.(string); ok {
			profileData.FullName = fullNameStr
		}
	}

	if bio, ok := data["bio"]; ok && bio != nil {
		if bioStr, ok := bio.(string); ok {
			profileData.Bio = bioStr

			// If we have a bio but no npub, try to extract npub from the bio
			if profileData.Npub == "" {
				npubIndex := strings.Index(bioStr, "npub1")
				if npubIndex >= 0 {
					// Extract the npub
					npubEnd := npubIndex
					for npubEnd < len(bioStr) && npubEnd < npubIndex+64 &&
						((bioStr[npubEnd] >= 'a' && bioStr[npubEnd] <= 'z') ||
							(bioStr[npubEnd] >= '0' && bioStr[npubEnd] <= '9')) {
						npubEnd++
					}
					if npubEnd > npubIndex {
						profileData.Npub = bioStr[npubIndex:npubEnd]
					}
				}
			}
		}
	}

	if location, ok := data["location"]; ok && location != nil {
		if locationStr, ok := location.(string); ok {
			profileData.Location = locationStr
		}
	}

	if website, ok := data["website"]; ok && website != nil {
		if websiteStr, ok := website.(string); ok {
			profileData.Website = websiteStr
		}
	}

	if joinDate, ok := data["join_date"]; ok && joinDate != nil {
		if joinDateStr, ok := joinDate.(string); ok {
			profileData.JoinDate = joinDateStr
		}
	}

	if tweetCount, ok := data["tweet_count"]; ok && tweetCount != nil {
		if tweetCountStr, ok := tweetCount.(string); ok {
			profileData.TweetCount = tweetCountStr
		} else if tweetCountFloat, ok := tweetCount.(float64); ok {
			profileData.TweetCount = fmt.Sprintf("%g", tweetCountFloat)
		}
	}

	if followingCount, ok := data["following_count"]; ok && followingCount != nil {
		if followingCountStr, ok := followingCount.(string); ok {
			profileData.FollowingCount = followingCountStr
		} else if followingCountFloat, ok := followingCount.(float64); ok {
			profileData.FollowingCount = fmt.Sprintf("%g", followingCountFloat)
		}
	}

	if likesCount, ok := data["likes_count"]; ok && likesCount != nil {
		if likesCountStr, ok := likesCount.(string); ok {
			profileData.LikesCount = likesCountStr
		} else if likesCountFloat, ok := likesCount.(float64); ok {
			profileData.LikesCount = fmt.Sprintf("%g", likesCountFloat)
		}
	}

	return profileData, nil
}

// saveProfileData saves profile data to a JSON file
func (s *Service) saveProfileData(profileData ProfileData, jsonOutputFile string) error {
	// Convert profile data to JSON
	jsonData, err := json.MarshalIndent(profileData, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to convert profile data to JSON: %w", err)
	}

	// Save JSON to file
	if err := os.WriteFile(jsonOutputFile, jsonData, 0644); err != nil {
		return fmt.Errorf("failed to save JSON to file: %w", err)
	}

	fmt.Printf("Profile data saved to %s\n", jsonOutputFile)
	return nil
}

// readProfileData reads profile data from a JSON file
func (s *Service) readProfileData(jsonPath string) (ProfileData, error) {
	var profileData ProfileData

	// Read the JSON file
	jsonData, err := os.ReadFile(jsonPath)
	if err != nil {
		return profileData, fmt.Errorf("failed to read JSON file: %w", err)
	}

	// Parse the JSON
	if err := json.Unmarshal(jsonData, &profileData); err != nil {
		return profileData, fmt.Errorf("failed to parse JSON: %w", err)
	}

	return profileData, nil
}
