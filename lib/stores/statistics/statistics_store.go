package statistics

import (
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/nbd-wtf/go-nostr"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
)

// StatisticsStore defines the interface for storing and retrieving statistical data.
type StatisticsStore interface {
	// Bitcoin-related statistics
	SaveBitcoinRate(rate float64) error
	GetBitcoinRatesLast30Days() ([]types.BitcoinRate, error)

	// Pending/Unconfirmed transactions
	SavePendingTransaction(transaction types.PendingTransaction) error
	GetPendingTransactionByID(id string) (*types.PendingTransaction, error)
	DeletePendingTransaction(txID string) error
	ReplaceTransaction(replaceRequest types.ReplaceTransactionRequest) error
	GetPendingTransactions() ([]types.PendingTransaction, error)

	// Event kinds and user profiles
	SaveEventKind(event *nostr.Event) error
	UpsertUserProfile(npubKey string, lightningAddr, dhtKey bool, createdAt time.Time) error
	DeleteEventByID(eventID string) error

	// User registration and authentication
	SignUpUser(npub string, password string) error
	ComparePasswords(hashedPassword, password string) error
	FindUserByNpub(npub string) (*types.AdminUser, error)
	UserExists() (bool, error)
	GetUserByID(userID uint) (types.AdminUser, error)

	// File-related statistics (photos, videos, etc.)
	SaveFile(root string, hash string, fileName string, mimeType string, leafCount int, size int64) error
	QueryFiles(criteria map[string]interface{}) ([]types.FileInfo, error)
	SaveTags(root string, leaf *merkle_dag.DagLeaf) error
	QueryTags(tags map[string]string) ([]string, error)
	FetchKindData() ([]types.AggregatedKindData, error)
	FetchKindTrendData(kindNumber int) ([]types.MonthlyKindData, error)

	// Wallet-related operations
	SaveWalletTransaction(tx types.WalletTransactions) error
	UpdateWalletBalance(walletName, balance string) error
	GetLatestWalletBalance() (types.WalletBalance, error)
	TransactionExists(address string, date time.Time, output string, value string) (bool, error)
	GetLatestWalletTransactions() ([]types.WalletTransactions, error)
	FetchWalletAddresses() ([]types.WalletAddress, error)
	SaveAddress(address *types.WalletAddress) error
	AddressExists(address string) (bool, error)
	GetLatestBitcoinRate() (types.BitcoinRate, error)
	UpdateBitcoinRate(rate float64) error
	SaveUnconfirmedTransaction(pendingTransaction *types.PendingTransaction) error
	CountAvailableAddresses() (int64, error)
	AllocateBitcoinAddress(npub string) (*types.Address, error)
	GetSubscriberByAddress(address string) (*types.SubscriberAddress, error)
	GetSubscriberByNpub(npub string) (*types.SubscriberAddress, error)
	SaveSubscriberAddress(address *types.SubscriberAddress) error
	WalletAddressExists(address string) (bool, error)
	SubscriberAddressExists(address string) (bool, error)
	GetSubscriberCredit(npub string) (int64, error)
	UpdateSubscriberCredit(npub string, creditSats int64) error

	// User challenge and token management
	SaveUserChallenge(userChallenge *types.UserChallenge) error
	GetUserChallenge(challenge string) (types.UserChallenge, error)
	MarkChallengeExpired(userChallenge *types.UserChallenge) error
	StoreActiveToken(activeToken *types.ActiveToken) error
	DeleteActiveToken(token uint) error
	FindUserByToken(token string) (*types.AdminUser, error)
	IsActiveToken(token string) (bool, error)

	// Statistics and storage stats
	FetchMonthlyStorageStats() ([]types.ActivityData, error)
	FetchNotesMediaStorageData() ([]types.BarChartData, error)
	FetchProfilesTimeSeriesData(startDate, endDate string) ([]types.TimeSeriesData, error)

	// Fetch counts for various file types
	FetchKindCount() (int, error)
	FetchFileCountByType(mimeType string) (int, error)
	FetchFilesByType(mimeType string, page int, pageSize int) ([]types.FileInfo, *types.PaginationMetadata, error)

	// Paid subscriber management
	GetPaidSubscribers() ([]types.PaidSubscriber, error)
	GetPaidSubscriberByNpub(npub string) (*types.PaidSubscriber, error)
	SavePaidSubscriber(subscriber *types.PaidSubscriber) error
	UpdatePaidSubscriber(subscriber *types.PaidSubscriber) error
	DeletePaidSubscriber(npub string) error
}
