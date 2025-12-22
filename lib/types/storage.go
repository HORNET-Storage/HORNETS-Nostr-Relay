// Storage and DAG related types
package types

import (
	"time"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
	"github.com/fxamacker/cbor/v2"
)

// HashBytes is a []byte type that can unmarshal from both CBOR bytes and strings
// for backwards compatibility with old data stored as strings
type HashBytes []byte

// UnmarshalCBOR implements custom CBOR unmarshaling to handle both string and bytes
func (h *HashBytes) UnmarshalCBOR(data []byte) error {
	// Try to unmarshal as bytes first
	var bytes []byte
	if err := cbor.Unmarshal(data, &bytes); err == nil {
		*h = bytes
		return nil
	}

	// Fall back to string (for backwards compatibility)
	var str string
	if err := cbor.Unmarshal(data, &str); err != nil {
		return err
	}
	*h = []byte(str)
	return nil
}

// LeafContent represents the content-addressed leaf data (stored once per unique hash)
// NOTE: ParentHash is NOT stored here because the same leaf can have different parents
// in different DAGs. Parent relationships are cached per-root separately.
// NOTE: Indexes removed to reduce write amplification - lookups use composite keys directly
type LeafContent struct {
	Hash              string
	ItemName          string
	Type              merkle_dag.LeafType
	ContentHash       []byte
	ClassicMerkleRoot []byte
	CurrentLinkCount  int
	LeafCount         int
	ContentSize       int64
	DagSize           int64
	Links             []string
	AdditionalData    map[string]string
}

// DagOwnership represents ownership/signature for a leaf within a DAG
// NOTE: Root and LeafHash indexes are needed for retrieval queries.
// PublicKey index removed to reduce write amplification during uploads.
type DagOwnership struct {
	Root      string `badgerhold:"index"`
	PublicKey string
	Signature string
	LeafHash  string `badgerhold:"index"`
}

// LeafParentCache caches the parent hash for a leaf within a specific DAG root
// This is needed because the same leaf can have different parents in different DAGs
// NOTE: Indexes removed - lookups use composite key "root:leafhash"
type LeafParentCache struct {
	RootHash   string
	LeafHash   string
	ParentHash string
}

// WrappedLeaf represents a leaf in the Merkle DAG structure (backward compatibility)
// This combines LeafContent + DagOwnership for convenience
// NOTE: Indexes removed to reduce write amplification
type WrappedLeaf struct {
	PublicKey         string
	Signature         string
	Root              string
	Hash              string
	ItemName          string
	Type              merkle_dag.LeafType
	ContentHash       []byte
	ClassicMerkleRoot []byte
	CurrentLinkCount  int
	LeafCount         int
	ContentSize       int64
	DagSize           int64
	Links             []string
	AdditionalData    map[string]string
}

// AdditionalDataEntry represents additional metadata for DAG entries
// NOTE: Indexes removed - lookups use composite key "root:pubkey:leafhash:key"
type AdditionalDataEntry struct {
	Hash  HashBytes
	Key   string
	Value string
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
