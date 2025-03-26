package contentmoderation

import (
	"errors"
	"time"
)

// ContentStatus represents the moderation status of a piece of content
type ContentStatus string

const (
	// StatusAwaiting indicates the content is awaiting moderation
	StatusAwaiting ContentStatus = "awaiting"

	// StatusProcessing indicates the content is currently being processed by the moderation service
	StatusProcessing ContentStatus = "processing"

	// StatusApproved indicates the content has been approved by moderation
	StatusApproved ContentStatus = "approved"

	// StatusRejected indicates the content has been rejected by moderation
	StatusRejected ContentStatus = "rejected"

	// StatusDeleted indicates rejected content has been deleted
	StatusDeleted ContentStatus = "deleted"
)

// EventStatus represents the safety status of an event regarding media references
type EventStatus string

const (
	// EventStatusSafe indicates an event has no problematic media references
	EventStatusSafe EventStatus = "safe"

	// EventStatusUnsafe indicates an event has at least one problematic media reference
	EventStatusUnsafe EventStatus = "unsafe"

	// EventStatusAwaiting indicates an event has media references awaiting moderation
	EventStatusAwaiting EventStatus = "awaiting"
)

// Custom errors
var (
	// ErrNoMediaWaiting is returned when there is no media waiting for moderation
	ErrNoMediaWaiting = errors.New("no media waiting for moderation")

	// ErrMediaAlreadyProcessed is returned when attempting to process media that has already been processed
	ErrMediaAlreadyProcessed = errors.New("media has already been processed")

	// ErrMediaNotFound is returned when looking up media that doesn't exist
	ErrMediaNotFound = errors.New("media not found")

	// ErrInvalidMediaData is returned when the media data is invalid or corrupt
	ErrInvalidMediaData = errors.New("invalid media data")

	// ErrExternalMediaTimeout is returned when fetching external media times out
	ErrExternalMediaTimeout = errors.New("external media fetch timeout")

	// ErrExternalMediaTooLarge is returned when external media exceeds size limits
	ErrExternalMediaTooLarge = errors.New("external media exceeds size limit")
)

// ErrAwaitingModeration is returned when an event contains media awaiting moderation
type ErrAwaitingModeration struct {
	EventID string
}

func (e *ErrAwaitingModeration) Error() string {
	return "event contains media awaiting moderation: " + e.EventID
}

// ModerationRecord represents the status and details of a piece of media content
type ModerationRecord struct {
	// DagRoot is the unique identifier for the media content (from Scionic)
	DagRoot string `json:"dag_root"`

	// ContentType is the MIME type of the media
	ContentType string `json:"content_type"`

	// FileSize is the size of the media file in bytes
	FileSize int64 `json:"file_size"`

	// UploadedBy is the public key of the user who uploaded the media
	UploadedBy string `json:"uploaded_by"`

	// UploadedAt is the time when the media was uploaded
	UploadedAt time.Time `json:"uploaded_at"`

	// Status is the current moderation status of the media
	Status ContentStatus `json:"status"`

	// ModeratedAt is the time when the media was moderated (if completed)
	ModeratedAt *time.Time `json:"moderated_at,omitempty"`

	// ModerationLevel is the content level determined by moderation (0-5)
	ModerationLevel int `json:"moderation_level,omitempty"`

	// ModerationData is the full data returned by the moderation API
	ModerationData interface{} `json:"moderation_data,omitempty"`

	// AttemptCount tracks the number of moderation attempts for this media
	AttemptCount int `json:"attempt_count"`

	// ProcessingError contains any error that occurred during processing
	ProcessingError string `json:"processing_error,omitempty"`

	// CreatedAt is the time when this record was created
	CreatedAt time.Time `json:"created_at"`

	// UpdatedAt is the time when this record was last updated
	UpdatedAt time.Time `json:"updated_at"`
}

// MediaReferenceInfo represents information about media referenced in events
type MediaReferenceInfo struct {
	// MediaURL is the full URL of the media
	MediaURL string `json:"media_url"`

	// MediaID is the unique identifier for the media (DagRoot for Scionic, filename for external)
	MediaID string `json:"media_id"`

	// Type describes how the media is referenced (content_url, imeta_tag, r_tag, etc.)
	Type string `json:"type"`

	// SourceType indicates the source of the media (scionic, external, etc.)
	SourceType string `json:"source_type"`

	// Metadata contains additional information about the media (dimensions, blurhash, etc.)
	Metadata map[string]string `json:"metadata,omitempty"`

	// IsModerated indicates if this media has been moderated
	IsModerated bool `json:"is_moderated"`

	// Status is the moderation status if available
	Status ContentStatus `json:"status,omitempty"`
}

// EventMediaReference links an event to a media reference
type EventMediaReference struct {
	// EventID is the ID of the event referencing the media
	EventID string `json:"event_id"`

	// MediaURL is the full URL of the referenced media
	MediaURL string `json:"media_url"`

	// MediaID is the unique identifier for the media
	MediaID string `json:"media_id"`

	// SourceType indicates the source of the media
	SourceType string `json:"source_type"`

	// ReferenceAt is when this reference was detected
	ReferenceAt time.Time `json:"reference_at"`
}

// EventModerationStatus represents the cached moderation status of an event
type EventModerationStatus struct {
	// EventID is the ID of the event
	EventID string `json:"event_id"`

	// Status is the moderation status of the event
	Status EventStatus `json:"status"`

	// CheckedAt is when the event was last checked
	CheckedAt time.Time `json:"checked_at"`
}

// ExternalMediaCache represents cached information about external media
type ExternalMediaCache struct {
	// URL is the full URL of the external media
	URL string `json:"url"`

	// Status is the moderation status of the media
	Status ContentStatus `json:"status"`

	// CheckedAt is when the media was last checked
	CheckedAt time.Time `json:"checked_at"`

	// FileHash is a hash of the media file content
	FileHash string `json:"file_hash"`

	// ContentType is the MIME type of the media
	ContentType string `json:"content_type"`
}

// ModerationResult represents the result from the moderation API
type ModerationResult struct {
	// ContentLevel is the severity level (0-5) determined by the API
	ContentLevel int `json:"content_level"`

	// IsExplicit indicates if the content is explicitly inappropriate
	IsExplicit bool `json:"is_explicit"`

	// Confidence is the confidence score for the classification
	Confidence float64 `json:"confidence"`

	// Category describes the type of content detected
	Category string `json:"category"`

	// Explanation provides a human-readable explanation
	Explanation string `json:"explanation"`

	// Decision indicates the recommended action (ALLOW, FLAG, BLOCK)
	Decision string `json:"decision"`

	// DetectedClasses contains specific classes detected in the content
	DetectedClasses []string `json:"detected_classes"`

	// ProcessingTime is how long the API took to process the content
	ProcessingTime float64 `json:"processing_time"`

	// IsVideo indicates if the content was a video
	IsVideo bool `json:"is_video"`

	// FrameResults contains results for individual frames if this is a video
	FrameResults []interface{} `json:"frame_results,omitempty"`
}
