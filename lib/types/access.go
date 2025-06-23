package types

import "time"

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
