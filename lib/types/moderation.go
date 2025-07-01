// Moderation and content blocking types
package types

import "time"

// PendingModeration represents an event waiting for media moderation
type PendingModeration struct {
	EventID   string    `json:"event_id"`   // Event ID as the primary identifier
	ImageURLs []string  `json:"image_urls"` // URLs of images or videos to moderate (kept as ImageURLs for backward compatibility)
	AddedAt   time.Time `json:"added_at"`   // Timestamp when added to queue
}

// BlockedEvent represents an event that has been blocked due to moderation
type BlockedEvent struct {
	EventID     string    `json:"event_id"`     // Event ID as the primary identifier
	Reason      string    `json:"reason"`       // Reason for blocking
	BlockedAt   time.Time `json:"blocked_at"`   // Timestamp when it was blocked
	RetainUntil time.Time `json:"retain_until"` // When to delete (typically 48hrs after blocking)
	HasDispute  bool      `json:"has_dispute"`  // Whether this event has an active dispute
}

// PendingDisputeModeration represents a dispute waiting for re-evaluation
type PendingDisputeModeration struct {
	DisputeID     string    `json:"dispute_id"`     // Dispute event ID
	TicketID      string    `json:"ticket_id"`      // Ticket event ID
	EventID       string    `json:"event_id"`       // Original blocked event ID
	MediaURL      string    `json:"media_url"`      // URL of the media to re-evaluate
	DisputeReason string    `json:"dispute_reason"` // Reason provided by the user for the dispute
	UserPubKey    string    `json:"user_pubkey"`    // Public key of the user who created the dispute
	AddedAt       time.Time `json:"added_at"`       // Timestamp when added to queue
}

// BlockedPubkey represents a pubkey that is blocked from connecting to the relay
type BlockedPubkey struct {
	Pubkey    string    `json:"pubkey" badgerhold:"key"`       // Pubkey as the primary identifier
	Reason    string    `json:"reason"`                        // Reason for blocking
	BlockedAt time.Time `json:"blocked_at" badgerhold:"index"` // Timestamp when it was blocked
}

// ModerationNotification represents a notification about moderated content
type ModerationNotification struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	PubKey       string    `gorm:"size:128;index" json:"pubkey"`         // User whose content was moderated
	EventID      string    `gorm:"size:128;uniqueIndex" json:"event_id"` // ID of the moderated event
	Reason       string    `gorm:"size:255" json:"reason"`               // Reason for blocking
	CreatedAt    time.Time `gorm:"autoCreateTime" json:"created_at"`     // When the notification was created
	IsRead       bool      `gorm:"default:false" json:"is_read"`         // Whether the notification has been read
	ContentType  string    `gorm:"size:64" json:"content_type"`          // Type of content (image/video)
	MediaURL     string    `gorm:"size:512" json:"media_url"`            // URL of the media that triggered moderation
	ThumbnailURL string    `gorm:"size:512" json:"thumbnail_url"`        // Optional URL for thumbnail
}

// ModerationStats represents statistics about moderated content
type ModerationStats struct {
	TotalBlocked      int        `json:"total_blocked"`       // Total number of blocked events
	TotalBlockedToday int        `json:"total_blocked_today"` // Number of events blocked today
	ByContentType     []TypeStat `json:"by_content_type"`     // Breakdown by content type
	ByUser            []UserStat `json:"by_user"`             // Top users with blocked content
	RecentReasons     []string   `json:"recent_reasons"`      // Recent blocking reasons
}

// ReportNotification represents a notification about content reported by users (kind 1984)
type ReportNotification struct {
	ID             uint      `gorm:"primaryKey" json:"id"`
	PubKey         string    `gorm:"size:128;index" json:"pubkey"`         // User whose content was reported
	EventID        string    `gorm:"size:128;uniqueIndex" json:"event_id"` // ID of the reported event
	ReportType     string    `gorm:"size:64" json:"report_type"`           // Type from NIP-56 (nudity, malware, etc.)
	ReportContent  string    `gorm:"size:512" json:"report_content"`       // Content field from the report event
	ReporterPubKey string    `gorm:"size:128" json:"reporter_pubkey"`      // First reporter's public key
	ReportCount    int       `gorm:"default:1" json:"report_count"`        // Number of reports for this content
	CreatedAt      time.Time `gorm:"autoCreateTime" json:"created_at"`     // When the report was first received
	UpdatedAt      time.Time `gorm:"autoUpdateTime" json:"updated_at"`     // When the report was last updated
	IsRead         bool      `gorm:"default:false" json:"is_read"`         // Whether the notification has been read
}

// ReportStats represents statistics about reported content
type ReportStats struct {
	TotalReported      int             `json:"total_reported"`       // Total number of reported events
	TotalReportedToday int             `json:"total_reported_today"` // Number of events reported today
	ByReportType       []TypeStat      `json:"by_report_type"`       // Breakdown by report type
	MostReported       []ReportSummary `json:"most_reported"`        // Most frequently reported content
}

// ReportSummary represents a summary of a reported event
type ReportSummary struct {
	EventID     string    `json:"event_id"`     // ID of the reported event
	PubKey      string    `json:"pubkey"`       // Author of the reported content
	ReportCount int       `json:"report_count"` // Number of times reported
	ReportType  string    `json:"report_type"`  // Type of report
	CreatedAt   time.Time `json:"created_at"`   // When first reported
}
