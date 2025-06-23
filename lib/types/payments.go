// Payment and subscription related types
package types

import "time"

// Subscriber represents a user subscription
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
