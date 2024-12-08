package lib

import (
	"bytes"
	"context"
	"io"
	"time"

	"github.com/gofiber/contrib/websocket"
	"github.com/golang-jwt/jwt/v4"
	"github.com/libp2p/go-libp2p/core/network"

	merkle_dag "github.com/HORNET-Storage/scionic-merkletree/dag"
)

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
	Count     int
	Leaf      merkle_dag.DagLeaf
	Parent    string
	Branch    *merkle_dag.ClassicTreeBranch
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
	From           string
	To             string
	IncludeContent bool
}

type DownloadFilter struct {
	Leaves         []string
	LeafRanges     []LeafLabelRange
	IncludeContent bool // IncludeContent from LeafLabelRange always overrides this
}

type QueryFilter struct {
	Buckets []string
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

func (FileInfo) TableName() string {
	return "file_info"
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
	Mode              string             `json:"mode"`
	Protocol          []string           `json:"protocol"`
	Chunked           []string           `json:"chunked"`
	Chunksize         string             `json:"chunksize"`
	MaxFileSize       int                `json:"maxFileSize"`
	MaxFileSizeUnit   string             `json:"maxFileSizeUnit"`
	SubscriptionTiers []SubscriptionTier `json:"subscription_tiers"`

	// Common type groups used for determining what types are considered audio, videos, images etc
	MimeTypeGroups map[string][]string

	// Whitelist will be disabled if this is empty allowing any file types to be stored
	MimeTypeWhitelist []string

	// Whitelist will be disabled if this is empty allowing any kind handlers available to work (will accept any kind if in unlimited mode)
	KindWhitelist []string

	// To be replaced by clearer definitions above
	/*
		IsKindsActive    bool     `json:"isKindsActive"`
		IsPhotosActive   bool     `json:"isPhotosActive"`
		IsVideosActive   bool     `json:"isVideosActive"`
		IsGitNestrActive bool     `json:"isGitNestrActive"`
		IsAudioActive    bool     `json:"isAudioActive"`
		Photos           []string `json:"photos"`
		Videos           []string `json:"videos"`
		GitNestr         []string `json:"gitNestr"`
		Audio            []string `json:"audio"`
		Kinds            []string `json:"kinds"`
		DynamicKinds     []string `json:"dynamicKinds"`
	*/
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
	UserID    uint   `gorm:"primaryKey"`
	Token     string `gorm:"size:512;uniqueIndex"`
	ExpiresAt time.Time
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
	Npub      string    `gorm:"size:128;uniqueIndex"`
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
	Status       string     `json:"status"`
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
	Npub              string    `json:"npub"`                // The unique public key of the subscriber
	Tier              string    `json:"tier"`                // The subscription tier the user has selected
	StartDate         time.Time `json:"start_date"`          // When the subscription started
	EndDate           time.Time `json:"end_date"`            // When the subscription ends
	Address           string    `json:"address"`             // The address associated with the subscription
	LastTransactionID string    `json:"last_transaction_id"` // The ID of the last processed transaction
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
}

type UserChallenge struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	Npub      string `gorm:"size:128;index"`
	Challenge string `gorm:"size:512;uniqueIndex"`
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
	DataLimit string
	Price     string
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
