package types

import "time"

type AllowedUser struct {
	Npub      string    `gorm:"primaryKey;size:128" json:"npub"`
	Tier      string    `gorm:"size:64" json:"tier"`
	CreatedAt time.Time `gorm:"autoCreateTime" json:"created_at"`
	CreatedBy string    `gorm:"size:128" json:"created_by"`
}
