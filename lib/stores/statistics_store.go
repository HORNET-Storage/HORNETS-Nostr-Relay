package stores

import (
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

// StatisticsStore defines the interface for storing and retrieving statistical data.
type StatisticsStore interface {
	// General initialization for the store
	InitStore(basepath string, args ...interface{}) error

	// Functions for statistics management
	SaveBitcoinRate(rate types.BitcoinRate) error
	GetBitcoinRatesLast30Days() ([]types.BitcoinRate, error)
	SavePendingTransaction(transaction types.PendingTransaction) error
	GetPendingTransactionByID(id string) (*types.PendingTransaction, error)
	UpsertUserProfile(npubKey string, lightningAddr, dhtKey bool, createdAt time.Time) error
	SaveEventKind(eventKind types.Kind) error
}
