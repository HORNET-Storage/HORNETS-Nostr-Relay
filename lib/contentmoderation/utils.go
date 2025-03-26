package contentmoderation

import (
	"encoding/json"
	"log"
	"strings"
)

// IsMediaMimeType checks if a MIME type is a media type (image or video)
func IsMediaMimeType(mimeType string) bool {
	// Check for empty mime type
	if mimeType == "" {
		return false
	}

	// Check for image types
	if strings.HasPrefix(mimeType, "image/") {
		return true
	}

	// Check for video types
	if strings.HasPrefix(mimeType, "video/") {
		return true
	}

	// Check for specific audio types that might have visual content
	audioTypes := []string{
		"audio/mp4",
		"audio/mpeg",
		"audio/ogg",
	}
	for _, t := range audioTypes {
		if mimeType == t {
			return true
		}
	}

	return false
}

// IsImageMimeType checks if a MIME type is an image type
func IsImageMimeType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "image/")
}

// IsVideoMimeType checks if a MIME type is a video type
func IsVideoMimeType(mimeType string) bool {
	return strings.HasPrefix(mimeType, "video/")
}

// SerializeModerationResult serializes a ModerationResult to JSON
func SerializeModerationResult(result *ModerationResult) (string, error) {
	if result == nil {
		return "{}", nil
	}

	data, err := json.Marshal(result)
	if err != nil {
		return "", err
	}

	return string(data), nil
}

// DeserializeModerationResult deserializes a JSON string to a ModerationResult
func DeserializeModerationResult(data string) (*ModerationResult, error) {
	if data == "" {
		return nil, nil
	}

	result := new(ModerationResult)
	err := json.Unmarshal([]byte(data), result)
	if err != nil {
		return nil, err
	}

	return result, nil
}

// StatusToString converts a ContentStatus to a human-readable string
func StatusToString(status ContentStatus) string {
	switch status {
	case StatusAwaiting:
		return "Awaiting Moderation"
	case StatusProcessing:
		return "Processing"
	case StatusApproved:
		return "Approved"
	case StatusRejected:
		return "Rejected"
	case StatusDeleted:
		return "Deleted"
	default:
		return string(status)
	}
}

// ContentLevelToString converts a content level to a human-readable description
func ContentLevelToString(level int) string {
	switch level {
	case 0:
		return "Appropriate"
	case 1:
		return "Suggestive (Clothed)"
	case 2:
		return "Revealing (Appropriate Context)"
	case 3:
		return "Revealing (Suggestive Context)"
	case 4:
		return "Borderline"
	case 5:
		return "Explicit"
	default:
		return "Unknown"
	}
}

// DecisionToString converts a decision to a human-readable description
func DecisionToString(decision string) string {
	switch strings.ToUpper(decision) {
	case "ALLOW":
		return "Allow (Appropriate Content)"
	case "FLAG":
		return "Flag (Questionable Content)"
	case "BLOCK":
		return "Block (Inappropriate Content)"
	default:
		return decision
	}
}

// LogModerationResult logs a moderation result
func LogModerationResult(dagRoot string, result *ModerationResult) {
	if result == nil {
		log.Printf("No moderation result for %s", dagRoot)
		return
	}

	log.Printf("Moderation result for %s: Level=%d (%s), Explicit=%v, Decision=%s",
		dagRoot,
		result.ContentLevel,
		ContentLevelToString(result.ContentLevel),
		result.IsExplicit,
		result.Decision)

	if result.Explanation != "" {
		log.Printf("Explanation: %s", result.Explanation)
	}
}

// GetLevelFromStatus gets a content level from a status
func GetLevelFromStatus(status ContentStatus) int {
	switch status {
	case StatusApproved:
		return 0 // Appropriate
	case StatusRejected, StatusDeleted:
		return 5 // Explicit
	case StatusAwaiting, StatusProcessing:
		return -1 // Unknown
	default:
		return -1
	}
}

// ShouldAllowBasedOnStatus determines if content should be allowed based on its status
func ShouldAllowBasedOnStatus(status ContentStatus) bool {
	switch status {
	case StatusApproved:
		return true
	case StatusRejected, StatusDeleted:
		return false
	case StatusAwaiting, StatusProcessing:
		// This depends on your policy - here we're conservative
		return false
	default:
		// Unknown status, be conservative
		return false
	}
}
