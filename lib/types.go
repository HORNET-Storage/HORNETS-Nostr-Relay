package lib

import (
	"time"

	"github.com/golang-jwt/jwt/v4"

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

type QueryMessage struct {
	QueryFilter map[string]string
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
	ID         uint `gorm:"primaryKey"`
	KindNumber int
	EventID    string
	Timestamp  time.Time `gorm:"autoCreateTime"`
	Size       float64   `gorm:"default:0"` // Size in MB
}

type Photo struct {
	ID        uint   `gorm:"primaryKey"`
	Hash      string `gorm:"uniqueIndex"`
	LeafCount int
	KindName  string
	Timestamp time.Time `gorm:"autoCreateTime"`
	Size      float64   `gorm:"default:0"` // Size in MB
}

type Video struct {
	ID        uint   `gorm:"primaryKey"`
	Hash      string `gorm:"uniqueIndex"`
	LeafCount int
	KindName  string
	Timestamp time.Time `gorm:"autoCreateTime"`
	Size      float64   `gorm:"default:0"` // Size in MB
}

type GitNestr struct {
	ID        uint `gorm:"primaryKey"`
	GitType   string
	EventID   string
	Timestamp time.Time `gorm:"autoCreateTime"`
	Size      float64   `gorm:"default:0"` // Size in MB
}

type WalletBalance struct {
	ID        uint      `gorm:"primaryKey"`
	Balance   string    `gorm:"not null"`
	Timestamp time.Time `gorm:"autoCreateTime"`
}

type WalletTransactions struct {
	ID      uint      `gorm:"primaryKey"`
	Address string    `gorm:"not null"`
	Date    time.Time `gorm:"not null"` // Date and time formatted like "2024-05-23 19:17:22"
	Output  string    `gorm:"not null"` // Output as a string
	Value   string    `gorm:"not null"` // Value as a float
}

type WalletAddress struct {
	ID      uint   `gorm:"primaryKey"`
	Index   string `gorm:"not null"`
	Address string `gorm:"not null;unique"`
}

type BitcoinRate struct {
	ID        uint      `gorm:"primaryKey"`
	Rate      float64   `gorm:"not null"`
	Timestamp time.Time `gorm:"autoUpdateTime"` // This will be updated each time the rate changes
}

type RelaySettings struct {
	Mode             string   `json:"mode"`
	Protocol         string   `json:"protocol"` // Added protocol
	Chunked          string   `json:"chunked"`
	Kinds            []string `json:"kinds"`
	Photos           []string `json:"photos"`
	Videos           []string `json:"videos"`
	GitNestr         []string `json:"gitNestr"`
	IsKindsActive    bool     `json:"isKindsActive"`
	IsPhotosActive   bool     `json:"isPhotosActive"`
	IsVideosActive   bool     `json:"isVideosActive"`
	IsGitNestrActive bool     `json:"isGitNestrActive"`
}

type TimeSeriesData struct {
	Month           string `json:"month"`
	Profiles        int    `json:"profiles"`
	LightningAddr   int    `json:"lightning_addr"`
	DHTKey          int    `json:"dht_key"`
	LightningAndDHT int    `json:"lightning_and_dht"`
}

type UserProfile struct {
	ID            uint      `gorm:"primaryKey"`
	NpubKey       string    `gorm:"uniqueIndex"`
	LightningAddr bool      `gorm:"default:false"`
	DHTKey        bool      `gorm:"default:false"`
	Timestamp     time.Time `gorm:"autoCreateTime"`
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

type User struct {
	ID        uint      `gorm:"primaryKey"`
	Password  string    // Store hashed passwords
	Npub      string    `gorm:"uniqueIndex"` // Add this field
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
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

type UserChallenge struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	Npub      string `gorm:"index"`
	Challenge string `gorm:"uniqueIndex"`
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
