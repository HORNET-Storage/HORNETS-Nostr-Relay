// Storage and DAG related types
package types

import (
	"time"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
)

// LeafContent represents the content-addressed leaf data (stored once per unique hash)
type LeafContent struct {
	Hash              string `badgerhold:"index"`
	ItemName          string `badgerhold:"index"`
	Type              merkle_dag.LeafType
	ContentHash       []byte
	ClassicMerkleRoot []byte
	CurrentLinkCount  int
	LeafCount         int
	ContentSize       int64
	DagSize           int64
	Links             []string
	ParentHash        string
	AdditionalData    map[string]string
}

// DagOwnership represents ownership/signature for a leaf within a DAG (indexed for queries)
type DagOwnership struct {
	Root      string `badgerhold:"index"` // Root DAG hash
	PublicKey string `badgerhold:"index"` // Who signed this
	Signature string
	LeafHash  string `badgerhold:"index"` // Which leaf this ownership record is for
}

// WrappedLeaf represents a leaf in the Merkle DAG structure (backward compatibility)
// This combines LeafContent + DagOwnership for convenience
type WrappedLeaf struct {
	PublicKey         string `badgerhold:"index"`
	Signature         string
	Root              string `badgerhold:"index"` // Root DAG hash for efficient querying
	Hash              string `badgerhold:"index"`
	ItemName          string `badgerhold:"index"`
	Type              merkle_dag.LeafType
	ContentHash       []byte
	ClassicMerkleRoot []byte
	CurrentLinkCount  int
	LeafCount         int
	ContentSize       int64
	DagSize           int64
	Links             []string
	ParentHash        string
	AdditionalData    map[string]string
}

// AdditionalDataEntry represents additional metadata for DAG entries
type AdditionalDataEntry struct {
	Hash  []byte
	Key   string `badgerhold:"index"`
	Value string `badgerhold:"index"`
}

// DagLeafData represents DAG leaf data with signature
type DagLeafData struct {
	PublicKey string
	Signature string
	Leaf      merkle_dag.DagLeaf
}

// DagData represents complete DAG data with signature
type DagData struct {
	PublicKey string
	Signature string
	Dag       merkle_dag.Dag
}

// UploadMessage represents a message for uploading DAG data
type UploadMessage struct {
	Root      string
	Packet    merkle_dag.SerializableTransmissionPacket
	PublicKey string
	Signature string
}

// DownloadMessage represents a message for downloading DAG data
type DownloadMessage struct {
	Root      string
	PublicKey string
	Signature string
	Filter    *DownloadFilter
}

// LeafLabelRange represents a range of leaf labels
type LeafLabelRange struct {
	From int
	To   int
}

// DownloadFilter represents filtering options for downloads
type DownloadFilter struct {
	LeafRanges     *LeafLabelRange
	LeafHashes     []string // Specific leaf hashes to download
	IncludeContent bool     // IncludeContent from LeafLabelRange always overrides this
}

// BlockData represents data for a block in the DAG
type BlockData struct {
	Leaf   merkle_dag.DagLeaf
	Branch merkle_dag.ClassicTreeBranch
}

// FileInfo represents metadata about stored files
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

// FileTag represents tags associated with files
type FileTag struct {
	ID    uint   `gorm:"primaryKey;type:INTEGER AUTO_INCREMENT"`
	Root  string `gorm:"size:64;index"`
	Key   string `gorm:"size:128;index"`
	Value string `gorm:"size:512;index"`
}

// DagContent represents content stored in DAG
type DagContent struct {
	Hash    string
	Content []byte
}

// BlobContent represents blob content with metadata
type BlobContent struct {
	Hash    string
	PubKey  string
	Content []byte
}

// BlobDescriptor represents metadata for a blob
type BlobDescriptor struct {
	URL      string `json:"url"`
	SHA256   string `json:"sha256"`
	Size     int64  `json:"size"`
	Type     string `json:"type,omitempty"`
	Uploaded int64  `json:"uploaded"`
}

// CacheMetaData represents metadata for cache entries
type CacheMetaData struct {
	LastAccessed string
}

// CacheData represents cached data
type CacheData struct {
	Keys []string
}
