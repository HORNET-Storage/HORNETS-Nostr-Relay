package lib

import (
	"time"

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

type RelaySettings struct {
	Mode             string   `json:"mode"`
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
