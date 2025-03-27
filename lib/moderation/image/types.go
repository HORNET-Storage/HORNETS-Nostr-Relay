package image

import (
	"time"
)

// ModerationType defines the type of moderation result
type ModerationType string

const (
	// Moderation decisions
	DecisionAllow ModerationType = "ALLOW" // Content levels 0-2 (appropriate or revealing in appropriate contexts)
	DecisionFlag  ModerationType = "FLAG"  // Content level 3 (revealing in suggestive contexts)
	DecisionBlock ModerationType = "BLOCK" // Content levels 4-5 (borderline or explicit)
)

// ContentLevel represents the severity level of image content
type ContentLevel int

const (
	// Content levels
	Level0_Appropriate          ContentLevel = 0 // No concerns (fully clothed, non-suggestive)
	Level1_SuggestiveClothed    ContentLevel = 1 // Suggestive poses but fully clothed
	Level2_RevealingAppropriate ContentLevel = 2 // Revealing clothing in appropriate contexts (swimwear, sports)
	Level3_RevealingSuggestive  ContentLevel = 3 // Revealing clothing in suggestive contexts
	Level4_Borderline           ContentLevel = 4 // Very revealing, potentially inappropriate but not explicit
	Level5_Explicit             ContentLevel = 5 // Nudity, sexual activity, pornographic content
)

// PendingModeration represents an event waiting for image moderation
type PendingModeration struct {
	EventID   string    `json:"event_id"`   // Event ID as the primary identifier
	ImageURLs []string  `json:"image_urls"` // URLs of images to moderate
	AddedAt   time.Time `json:"added_at"`   // Timestamp when added to queue
}

// ModerationResponse represents the response from the image moderation API
type ModerationResponse struct {
	ContentLevel        int      `json:"content_level"`         // 0-5 severity level
	IsExplicit          bool     `json:"is_explicit"`           // Whether content is considered explicit
	Confidence          float64  `json:"confidence"`            // Confidence level of the detection
	Category            string   `json:"category"`              // Category of detected content
	Explanation         string   `json:"explanation"`           // Human-readable explanation
	Decision            string   `json:"decision"`              // "ALLOW", "FLAG", or "BLOCK"
	DetectedClasses     []string `json:"detected_classes"`      // Types of content detected
	NudenetDetections   any      `json:"nudenet_detections"`    // Detection details
	ContextDetected     string   `json:"context_detected"`      // Context information
	LlamaVisionUsed     bool     `json:"llama_vision_used"`     // Whether LLaMA vision was used
	LlamaVisionResponse any      `json:"llama_vision_response"` // LLaMA vision response
	ProcessingTime      float64  `json:"processing_time"`       // Time taken to process the image
	ModerationMode      string   `json:"moderation_mode"`       // Mode of moderation used
	IsVideo             bool     `json:"is_video"`              // Whether the content is a video
	FrameCount          *int     `json:"frame_count"`           // Number of frames if video
	FrameResults        any      `json:"frame_results"`         // Frame-by-frame results if video
}

// GetDecision converts the API decision string to a ModerationType
func (m *ModerationResponse) GetDecision() ModerationType {
	switch m.Decision {
	case "ALLOW":
		return DecisionAllow
	case "FLAG":
		return DecisionFlag
	case "BLOCK":
		return DecisionBlock
	default:
		// Default to FLAG for unknown values
		return DecisionFlag
	}
}

// ShouldBlock returns true if the content should be blocked
func (m *ModerationResponse) ShouldBlock() bool {
	return m.GetDecision() == DecisionBlock
}
