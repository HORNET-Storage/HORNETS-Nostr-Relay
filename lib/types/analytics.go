// Analytics and statistics types
package types

import "time"

// PaginationMetadata represents pagination information
type PaginationMetadata struct {
	CurrentPage int   `json:"currentPage"`
	PageSize    int   `json:"pageSize"`
	TotalItems  int64 `json:"totalItems"`
	TotalPages  int   `json:"totalPages"`
	HasNext     bool  `json:"hasNext"`
	HasPrevious bool  `json:"hasPrevious"`
}

// TimeSeriesData represents time series analytics data
type TimeSeriesData struct {
	Month           string `json:"month"`
	Profiles        int    `json:"profiles"`
	LightningAddr   int    `json:"lightning_addr"`
	DHTKey          int    `json:"dht_key"`
	LightningAndDHT int    `json:"lightning_and_dht"`
}

// ActivityData represents activity analytics data
type ActivityData struct {
	Month   string  `json:"month"`
	TotalGB float64 `json:"total_gb"`
}

// BarChartData represents bar chart data for analytics
type BarChartData struct {
	Month   string  `json:"month"`
	NotesGB float64 `json:"notes_gb"`
	MediaGB float64 `json:"media_gb"`
}

// AggregatedKindData represents aggregated data by event kind
type AggregatedKindData struct {
	KindNumber int     `json:"kindNumber"`
	KindCount  int     `json:"kindCount"`
	TotalSize  float64 `json:"totalSize"`
}

// KindData represents data for a specific event kind
type KindData struct {
	Month            string
	Size             float64
	TimestampHornets time.Time
}

// MonthlyKindData represents monthly aggregated kind data
type MonthlyKindData struct {
	Month     string  `json:"month"`
	TotalSize float64 `json:"totalSize"`
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
