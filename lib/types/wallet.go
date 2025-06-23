// Bitcoin wallet and transaction types
package types

import "time"

// WalletBalance represents the current wallet balance
type WalletBalance struct {
	ID               uint      `gorm:"primaryKey"`
	Balance          string    `gorm:"not null"`
	TimestampHornets time.Time `gorm:"autoCreateTime"`
}

// WalletTransactions represents wallet transaction history
type WalletTransactions struct {
	ID      uint      `gorm:"primaryKey"`
	Address string    `gorm:"not null"`
	Date    time.Time `gorm:"not null"` // Date and time formatted like "2024-05-23 19:17:22"
	Output  string    `gorm:"not null"` // Output as a string
	Value   string    `gorm:"not null"` // Value as a float
}

// WalletAddress represents a wallet address
type WalletAddress struct {
	ID           uint   `gorm:"primaryKey"`
	IndexHornets string `gorm:"not null"`
	Address      string `gorm:"not null;unique"`
}

// BitcoinRate represents the current Bitcoin exchange rate
type BitcoinRate struct {
	ID               uint      `gorm:"primaryKey"`
	Rate             string    `gorm:"not null"`
	TimestampHornets time.Time `gorm:"autoUpdateTime"` // This will be updated each time the rate changes
}

// PendingTransaction represents a pending Bitcoin transaction
type PendingTransaction struct {
	ID               uint      `gorm:"primaryKey"`
	TxID             string    `gorm:"not null;size:128;uniqueIndex" json:"txid"`
	FeeRate          int       `gorm:"not null" json:"feeRate"`
	Amount           int       `gorm:"not null" json:"amount"`
	RecipientAddress string    `gorm:"not null" json:"recipient_address"`
	TimestampHornets time.Time `gorm:"not null" json:"timestamp"`
	EnableRBF        bool      `gorm:"not null" json:"enable_rbf"` // New field for RBF
}

// ReplaceTransactionRequest represents a request to replace a transaction
type ReplaceTransactionRequest struct {
	OriginalTxID     string `json:"original_tx_id"`
	NewTxID          string `json:"new_tx_id"`
	NewFeeRate       int    `json:"new_fee_rate"`
	Amount           int    `json:"amount"`
	RecipientAddress string `json:"recipient_address"`
}

// Address represents an address structure to be stored in Graviton
type Address struct {
	IndexHornets string     `json:"index,string"` // Use string tag to handle string-encoded integers
	Address      string     `json:"address"`
	WalletName   string     `json:"wallet_name"`
	Status       string     `json:"status" badgerhold:"index"`
	AllocatedAt  *time.Time `json:"allocated_at,omitempty"`
	Npub         string     `json:"npub,omitempty"`
}

// AddressResponse represents a response containing address information
type AddressResponse struct {
	IndexHornets string `json:"index"`
	Address      string `json:"address"`
}
