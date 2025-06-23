// User authentication and management types
package types

import (
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// UserProfile represents a user profile
type UserProfile struct {
	ID               uint      `gorm:"primaryKey"`
	NpubKey          string    `gorm:"size:128;uniqueIndex"`
	LightningAddr    bool      `gorm:"default:false"`
	DHTKey           bool      `gorm:"default:false"`
	TimestampHornets time.Time `gorm:"autoCreateTime"`
}

// ActiveToken represents an active authentication token
type ActiveToken struct {
	ID        uint   `gorm:"primaryKey;type:INTEGER AUTO_INCREMENT"`
	UserID    uint   `gorm:"uniqueIndex"`
	Token     string `gorm:"size:512;uniqueIndex"` // Maximum allowed size for indexed columns
	ExpiresAt string `gorm:"type:VARCHAR[64]"`     // Changed to string to store formatted time
}

// AdminUser represents an admin user
type AdminUser struct {
	ID        uint      `gorm:"primaryKey"`
	Pass      string    // Store hashed passwords
	Npub      string    `gorm:"size:128"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
	UpdatedAt time.Time `gorm:"autoUpdateTime"`
}

// UserChallenge represents a user authentication challenge
type UserChallenge struct {
	ID        uint   `gorm:"primaryKey"`
	UserID    uint   `gorm:"index"`
	Npub      string `gorm:"size:128;index"`
	Challenge string `gorm:"size:512"`
	Hash      string
	Expired   bool      `gorm:"default:false"`
	CreatedAt time.Time `gorm:"autoCreateTime"`
}

// LoginRequest represents a login request
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// SignUpRequest represents a signup request
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
