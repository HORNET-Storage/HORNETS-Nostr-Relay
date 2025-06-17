package lib

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/golang-jwt/jwt/v4"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/nbd-wtf/go-nostr"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
)

type WrappedLeaf struct {
	PublicKey         string `badgerhold:"index"`
	Signature         string
	Hash              string `badgerhold:"index"`
	ItemName          string `badgerhold:"index"`
	Type              merkle_dag.LeafType
	ContentHash       []byte
	ClassicMerkleRoot []byte
	CurrentLinkCount  int
	LatestLabel       string
	LeafCount         int
	Links             map[string]string
	ParentHash        string
	AdditionalData    map[string]string
}

type AdditionalDataEntry struct {
	Hash  string
	Key   string `badgerhold:"index"`
	Value string
}

type TagEntry struct {
	EventID  string
	TagName  string
	TagValue string
}

type NostrEvent struct {
	ID        string `badgerhold:"index"`
	PubKey    string `badgerhold:"index"`
	CreatedAt nostr.Timestamp
	Kind      string `badgerhold:"index"`
	Tags      nostr.Tags
	Content   string
	Sig       string

	// Extra fields for serialization - this field is used indirectly during JSON serialization
	// and shouldn't be removed even though it appears unused in static analysis
	Extra map[string]any `json:"-"` // Fields will be added to the parent object during serialization
}

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

type DagLeafData struct {
	PublicKey string
	Signature string
	Leaf      merkle_dag.DagLeaf
}

type DagData struct {
	PublicKey string
	Signature string
	Dag       merkle_dag.Dag
}

type Stream interface {
	Read(p []byte) (int, error)
	Write(p []byte) (int, error)
	Close() error
	Context() context.Context
}

type UploadMessage struct {
	Root      string
	Packet    merkle_dag.SerializableTransmissionPacket
	PublicKey string
	Signature string
}

type DownloadMessage struct {
	Root      string
	PublicKey string
	Signature string
	Filter    *DownloadFilter
}

type LeafLabelRange struct {
	From int
	To   int
}

type DownloadFilter struct {
	LeafRanges     *LeafLabelRange
	IncludeContent bool // IncludeContent from LeafLabelRange always overrides this
}

type QueryFilter struct {
	Names   []string
	PubKeys []string
	Tags    map[string]string
}

type QueryMessage struct {
	QueryFilter map[string]string
}

type AdvancedQueryMessage struct {
	Filter QueryFilter
}

type QueryResponse struct {
	Hashes []string
}

type BlockData struct {
	Leaf   merkle_dag.DagLeaf
	Branch merkle_dag.ClassicTreeBranch
}

type ResponseMessage struct {
	Ok bool
}

type ErrorMessage struct {
	Message string
}

type CacheMetaData struct {
	LastAccessed string
}

type CacheData struct {
	Keys []string
}

type Kind struct {
	ID               uint `gorm:"primaryKey"`
	KindNumber       int
	EventID          string
	TimestampHornets time.Time `gorm:"autoCreateTime"`
	Size             float64
}

type FileInfo struct {
	ID               uint   `gorm:"primaryKey"`
	Root             string `gorm:"size:64;index"`
	Hash             string `gorm:"size:64;uniqueIndex"`
	FileName         string
	MimeType         string `gorm:"size:64;index"`
	LeafCount        int
	Size             int64     `gorm:"default:0"`
	TimestampHornets time.Time `gorm:"autoCreateTime"`
}

type FileTag struct {
	ID    uint   `gorm:"primaryKey;type:INTEGER AUTO_INCREMENT"`
	Root  string `gorm:"size:64;index"`
	Key   string `gorm:"size:128;index"`
	Value string `gorm:"size:512;index"`
}

type PaginationMetadata struct {
	CurrentPage int   `json:"currentPage"`
	PageSize    int   `json:"pageSize"`
	TotalItems  int64 `json:"totalItems"`
	TotalPages  int   `json:"totalPages"`
	HasNext     bool  `json:"hasNext"`
	HasPrevious bool  `json:"hasPrevious"`
}

type WalletBalance struct {
	ID               uint      `gorm:"primaryKey"`
	Balance          string    `gorm:"not null"`
	TimestampHornets time.Time `gorm:"autoCreateTime"`
}

type WalletTransactions struct {
	ID      uint      `gorm:"primaryKey"`
	Address string    `gorm:"not null"`
	Date    time.Time `gorm:"not null"` // Date and time formatted like "2024-05-23 19:17:22"
	Output  string    `gorm:"not null"` // Output as a string
	Value   string    `gorm:"not null"` // Value as a float
}

type WalletAddress struct {
	ID           uint   `gorm:"primaryKey"`
	IndexHornets string `gorm:"not null"`
	Address      string `gorm:"not null;unique"`
}

type BitcoinRate struct {
	ID               uint      `gorm:"primaryKey"`
	Rate             string    `gorm:"not null"`
	TimestampHornets time.Time `gorm:"autoUpdateTime"` // This will be updated each time the rate changes
}

type RelaySettings struct {
	Mode                string             `json:"mode" mapstructure:"mode"`
	Protocol            []string           `json:"protocol" mapstructure:"protocol"`
	Chunked             []string           `json:"chunked" mapstructure:"chunked"`
	Chunksize           string             `json:"chunksize" mapstructure:"chunksize"`
	MaxFileSize         int                `json:"maxfilesize" mapstructure:"maxfilesize"`
	MaxFileSizeUnit     string             `json:"maxfilesizeunit" mapstructure:"maxfilesizeunit"`
	IsFileStorageActive bool               `json:"isFileStorageActive" mapstructure:"isFileStorageActive"`
	SubscriptionTiers   []SubscriptionTier `json:"subscription_tiers" mapstructure:"subscription_tiers"`
	FreeTierEnabled     bool               `json:"freeTierEnabled" mapstructure:"freeTierEnabled"`
	FreeTierLimit       string             `json:"freeTierLimit" mapstructure:"freeTierLimit"`
	ModerationMode      string             `json:"moderationMode" mapstructure:"moderationMode"` // "strict" or "passive"
	LastUpdated         int64              `json:"lastUpdated" mapstructure:"lastUpdated"`
	MimeTypeGroups      map[string][]string
	MimeTypeWhitelist   []string
	KindWhitelist       []string
}

type TimeSeriesData struct {
	Month           string `json:"month"`
	Profiles        int    `json:"profiles"`
	LightningAddr   int    `json:"lightning_addr"`
	DHTKey          int    `json:"dht_key"`
	LightningAndDHT int    `json:"lightning_and_dht"`
}

type UserProfile struct {
	ID               uint      `gorm:"primaryKey"`
	NpubKey          string    `gorm:"size:128;uniqueIndex"`
	LightningAddr    bool      `gorm:"default:false"`
	DHTKey           bool      `gorm:"default:false"`
	TimestampHornets time.Time `gorm:"autoCreateTime"`
}

type ActiveToken struct {
	ID        uint   `gorm:"primaryKey;type:INTEGER AUTO_INCREMENT"`
	UserID    uint   `gorm:"uniqueIndex"`
	Token     string `gorm:"size:512;uniqueIndex"` // Maximum allowed size for indexed columns
	ExpiresAt string `gorm:"type:VARCHAR[64]"`     // Changed to string to store formatted time
}

type ActivityData struct {
	Month   string  `json:"month"`
	TotalGB float64 `json:"total_gb"`
}

type BarChartData struct {
	Month   string  `json:"month"`
	NotesGB float64 `json:"notes_gb"`
	MediaGB float64 `json:"media_gb"`
}

type AdminUser struct {
	ID        uint      `gorm:"primaryKey"`
	Pass      string    // Store hashed passwords
	Npub      string    `gorm:"size:128"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

type PendingTransaction struct {
	ID               uint      `gorm:"primaryKey"`
	TxID             string    `gorm:"not null;size:128;uniqueIndex" json:"txid"`
	FeeRate          int       `gorm:"not null" json:"feeRate"`
	Amount           int       `gorm:"not null" json:"amount"`
	RecipientAddress string    `gorm:"not null" json:"recipient_address"`
	TimestampHornets time.Time `gorm:"not null" json:"timestamp"`
	EnableRBF        bool      `gorm:"not null" json:"enable_rbf"` // New field for RBF
}

type ReplaceTransactionRequest struct {
	OriginalTxID     string `json:"original_tx_id"`
	NewTxID          string `json:"new_tx_id"`
	NewFeeRate       int    `json:"new_fee_rate"`
	Amount           int    `json:"amount"`
	RecipientAddress string `json:"recipient_address"`
}

// Address structure to be stored in Graviton
type Address struct {
	IndexHornets string     `json:"index,string"` // Use string tag to handle string-encoded integers
	Address      string     `json:"address"`
	WalletName   string     `json:"wallet_name"`
	Status       string     `json:"status" badgerhold:"index"`
	AllocatedAt  *time.Time `json:"allocated_at,omitempty"`
	Npub         string     `json:"npub,omitempty"`
}

// type User struct {
// 	ID        uint `gorm:"primaryKey"`
// 	FirstName string
// 	LastName  string
// 	Email     string    `gorm:"uniqueIndex"`
// 	Password  string    // Store hashed passwords
// 	Npub      string    `gorm:"uniqueIndex"` // Add this field
// 	CreatedAt time.Time `gorm:"autoCreateTime"`
// 	UpdatedAt time.Time `gorm:"autoUpdateTime"`
// }

type Subscriber struct {
	Npub              string    `json:"npub" badgerhold:"index"`    // The unique public key of the subscriber
	Tier              string    `json:"tier"`                       // The subscription tier the user has selected
	StartDate         time.Time `json:"start_date"`                 // When the subscription started
	EndDate           time.Time `json:"end_date"`                   // When the subscription ends
	Address           string    `json:"address" badgerhold:"index"` // The address associated with the subscription
	LastTransactionID string    `json:"last_transaction_id"`        // The ID of the last processed transaction
}

// SubscriberAddress represents the GORM-compatible model for storing addresses
type SubscriberAddress struct {
	ID           uint       `gorm:"primaryKey"`
	IndexHornets string     `gorm:"not null"`
	Address      string     `gorm:"not null;size:128;unique"`
	WalletName   string     `gorm:"not null"`
	Status       string     `gorm:"default:'available'"`
	AllocatedAt  *time.Time `gorm:"default:null"`
	Npub         *string    `gorm:"type:text;unique"` // Pointer type and unique constraint
	CreditSats   int64      `gorm:"default:0"`        // Track accumulated satoshis that haven't reached a tier
}

type UserChallenge struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	Npub      string `gorm:"size:128;index"`
	Challenge string `gorm:"size:512"`
	Hash      string
	Expired   bool      `gorm:"default:false"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type SignUpRequest struct {
	FirstName string `json:"firstName"`
	LastName  string `json:"lastName"`
	Email     string `json:"email"`
	Password  string `json:"password"`
}

type BlobDescriptor struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Type     string `json:"type,omitempty"`
	Uploaded int64  `json:"uploaded"`
}

// LoginPayload represents the structure of the login request payload
type LoginPayload struct {
	Npub     string `json:"npub"`
	Password string `json:"password"`
}

// JWTClaims represents the structure of the JWT claims
type JWTClaims struct {
	UserID uint   `json:"user_id"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

type Libp2pStream struct {
	Stream network.Stream
	Ctx    context.Context
}

type SubscriptionTier struct {
	DataLimit string `json:"datalimit" mapstructure:"datalimit"`
	Price     string `json:"price" mapstructure:"price"`
}

func (ls *Libp2pStream) Read(msg []byte) (int, error) {
	return ls.Stream.Read(msg)
}

func (ls *Libp2pStream) Write(msg []byte) (int, error) {
	return ls.Stream.Write(msg)
}

func (ls *Libp2pStream) Close() error {
	return ls.Stream.Close()
}

func (ls *Libp2pStream) Context() context.Context {
	return ls.Ctx
}

type WebSocketStream struct {
	Conn        *websocket.Conn
	Ctx         context.Context
	writeBuffer bytes.Buffer
}

type AggregatedKindData struct {
	KindNumber int     `json:"kindNumber"`
	KindCount  int     `json:"kindCount"`
	TotalSize  float64 `json:"totalSize"`
}

type KindData struct {
	Month            string
	Size             float64
	TimestampHornets time.Time
}

type MonthlyKindData struct {
	Month     string  `json:"month"`
	TotalSize float64 `json:"totalSize"`
}

func NewWebSocketStream(conn *websocket.Conn, ctx context.Context) *WebSocketStream {
	return &WebSocketStream{
		Conn: conn,
		Ctx:  ctx,
	}
}

func (ws *WebSocketStream) Read(msg []byte) (int, error) {
	_, reader, err := ws.Conn.ReadMessage()
	if err != nil {
		return 0, err
	}
	return io.ReadFull(bytes.NewReader(reader), msg)
}

func (ws *WebSocketStream) Write(msg []byte) (int, error) {
	ws.writeBuffer.Write(msg)
	return len(msg), nil
}

func (ws *WebSocketStream) Flush() error {
	err := ws.Conn.WriteMessage(websocket.BinaryMessage, ws.writeBuffer.Bytes())
	if err != nil {
		return err
	}
	ws.writeBuffer.Reset()
	return nil
}

func (ws *WebSocketStream) Close() error {
	return ws.Conn.Close()
}

func (ws *WebSocketStream) Context() context.Context {
	return ws.Ctx
}

type AddressResponse struct {
	IndexHornets string `json:"index"`
	Address      string `json:"address"`
}

type DagContent struct {
	Hash    string
	Content []byte
}

type BlobContent struct {
	Hash    string
	PubKey  string
	Content []byte
}

// PaidSubscriber represents a user with an active paid subscription
type PaidSubscriber struct {
	ID               uint      `gorm:"primaryKey"`
	Npub             string    `gorm:"size:128;uniqueIndex"` // Unique public key of the subscriber
	Tier             string    `gorm:"not null"`             // Subscription tier (e.g. "1 GB per month")
	ExpirationDate   time.Time `gorm:"not null"`             // When the subscription expires
	StorageBytes     int64     `gorm:"default:0"`            // Total storage allocated in bytes
	UsedBytes        int64     `gorm:"default:0"`            // Currently used storage in bytes
	TimestampHornets time.Time `gorm:"autoCreateTime"`       // When the record was created
	UpdatedAt        time.Time `gorm:"autoUpdateTime"`       // When the record was last updated
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

// PaymentNotification represents a notification about a payment/subscription event
type PaymentNotification struct {
	ID               uint      `gorm:"primaryKey" json:"id"`
	PubKey           string    `gorm:"size:128;index" json:"pubkey"`           // Subscriber's public key
	TxID             string    `gorm:"size:128;index" json:"tx_id"`            // Transaction ID
	Amount           int64     `gorm:"not null" json:"amount"`                 // Amount in satoshis
	SubscriptionTier string    `gorm:"size:64" json:"subscription_tier"`       // Tier purchased (e.g. "5GB")
	IsNewSubscriber  bool      `gorm:"default:false" json:"is_new_subscriber"` // First time subscriber?
	ExpirationDate   time.Time `json:"expiration_date"`                        // When subscription expires
	CreatedAt        time.Time `gorm:"autoCreateTime" json:"created_at"`       // When the notification was created
	IsRead           bool      `gorm:"default:false" json:"is_read"`           // Whether notification is read
}

// PaymentStats represents statistics about payments and subscriptions
type PaymentStats struct {
	TotalRevenue        int64       `json:"total_revenue"`         // Total sats received
	RevenueToday        int64       `json:"revenue_today"`         // Sats received today
	ActiveSubscribers   int         `json:"active_subscribers"`    // Currently active subs
	NewSubscribersToday int         `json:"new_subscribers_today"` // New subscribers today
	ByTier              []TierStat  `json:"by_tier"`               // Breakdown by tier
	RecentTransactions  []TxSummary `json:"recent_transactions"`   // Recent payments
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

// TierStat represents statistics for a specific subscription tier
type TierStat struct {
	Tier    string `json:"tier"`    // Subscription tier name
	Count   int    `json:"count"`   // Number of subscribers
	Revenue int64  `json:"revenue"` // Total revenue from this tier
}

// TxSummary represents a simplified transaction summary
type TxSummary struct {
	PubKey string    `json:"pubkey"` // Subscriber's public key
	Amount int64     `json:"amount"` // Amount in satoshis
	Tier   string    `json:"tier"`   // Tier purchased
	Date   time.Time `json:"date"`   // Transaction date
}

// TypeStat represents statistics for a specific content type
type TypeStat struct {
	Type  string `json:"type"`  // Content type (image/video)
	Count int    `json:"count"` // Number of items
}

// UserStat represents moderation statistics for a specific user
type UserStat struct {
	PubKey string `json:"pubkey"` // User public key
	Count  int    `json:"count"`  // Number of blocked items
}

// AllowedReadNpub represents an NPUB allowed to read from the relay
type AllowedReadNpub struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	Npub     string    `gorm:"size:128;uniqueIndex" json:"npub"`
	TierName string    `gorm:"size:64" json:"tier_name"` // Manual tier assignment for exclusive mode
	AddedAt  time.Time `gorm:"autoCreateTime" json:"added_at"`
	AddedBy  string    `gorm:"size:128" json:"added_by"` // Who added this NPUB
}

// AllowedWriteNpub represents an NPUB allowed to write to the relay
type AllowedWriteNpub struct {
	ID       uint      `gorm:"primaryKey" json:"id"`
	Npub     string    `gorm:"size:128;uniqueIndex" json:"npub"`
	TierName string    `gorm:"size:64" json:"tier_name"` // Manual tier assignment for exclusive mode
	AddedAt  time.Time `gorm:"autoCreateTime" json:"added_at"`
	AddedBy  string    `gorm:"size:128" json:"added_by"` // Who added this NPUB
}
