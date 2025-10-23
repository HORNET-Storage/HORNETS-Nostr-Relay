package statistics

import (
	"time"

	libtypes "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/nbd-wtf/go-nostr"

	merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/v2/dag"
)

// StatisticsStore defines the interface for storing and retrieving statistical data.
type StatisticsStore interface {
	Close() error

	// Bitcoin-related statistics
	SaveBitcoinRate(rate float64) error
	GetBitcoinRates(days int) ([]types.BitcoinRate, error)

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
	FetchKindData() ([]libtypes.AggregatedKindData, error)
	FetchKindTrendData(kindNumber int) ([]libtypes.MonthlyKindData, error)

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
	AllocateBitcoinAddress(npub string) (*libtypes.Address, error)
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
	FetchMonthlyStorageStats() ([]libtypes.ActivityData, error)
	FetchNotesMediaStorageData() ([]libtypes.BarChartData, error)
	FetchProfilesTimeSeriesData(startDate, endDate string) ([]libtypes.TimeSeriesData, error)

	// Fetch counts for various file types
	FetchKindCount() (int, error)
	FetchFileCountByType(mimeType string) (int, error)
	FetchFilesByType(mimeType string, page int, pageSize int) ([]libtypes.FileInfo, *libtypes.PaginationMetadata, error)

	// Paid subscriber management
	GetPaidSubscribers() ([]types.PaidSubscriber, error)
	GetPaidSubscriberByNpub(npub string) (*types.PaidSubscriber, error)
	SavePaidSubscriber(subscriber *types.PaidSubscriber) error
	UpdatePaidSubscriber(subscriber *types.PaidSubscriber) error
	DeletePaidSubscriber(npub string) error

	// Moderation notification management
	CreateModerationNotification(notification *types.ModerationNotification) error
	GetAllModerationNotifications(page, limit int) ([]types.ModerationNotification, *types.PaginationMetadata, error)
	GetUserModerationNotifications(pubkey string, page, limit int) ([]types.ModerationNotification, *types.PaginationMetadata, error)
	GetUnreadModerationNotifications(page, limit int) ([]types.ModerationNotification, *types.PaginationMetadata, error)
	MarkNotificationAsRead(id uint) error
	MarkAllNotificationsAsRead(pubkey string) error
	DeleteModerationNotification(id uint) error

	// Moderation statistics
	GetModerationStats() (*types.ModerationStats, error)
	GetBlockedContentCount() (int, error)
	GetTodayBlockedContentCount() (int, error)
	GetBlockedContentByType() ([]libtypes.TypeStat, error)
	GetBlockedContentByUser(limit int) ([]libtypes.UserStat, error)
	GetRecentBlockingReasons(limit int) ([]string, error)

	// Payment notification management
	CreatePaymentNotification(notification *types.PaymentNotification) error
	GetAllPaymentNotifications(page, limit int) ([]types.PaymentNotification, *types.PaginationMetadata, error)
	GetUserPaymentNotifications(pubkey string, page, limit int) ([]types.PaymentNotification, *types.PaginationMetadata, error)
	GetUnreadPaymentNotifications(page, limit int) ([]types.PaymentNotification, *types.PaginationMetadata, error)
	MarkPaymentNotificationAsRead(id uint) error
	MarkAllPaymentNotificationsAsRead() error
	DeletePaymentNotification(id uint) error

	// Payment statistics
	GetPaymentStats() (*types.PaymentStats, error)
	GetTotalRevenue() (int64, error)
	GetTodayRevenue() (int64, error)
	GetActiveSubscribersCount() (int, error)
	GetNewSubscribersToday() (int, error)
	GetRevenueByTier() ([]libtypes.TierStat, error)
	GetRecentTransactions(limit int) ([]libtypes.TxSummary, error)

	// Report notification management
	CreateReportNotification(notification *types.ReportNotification) error
	GetReportNotificationByEventID(eventID string) (*types.ReportNotification, error)
	UpdateReportCount(eventID string) error
	GetAllReportNotifications(page, limit int) ([]types.ReportNotification, *types.PaginationMetadata, error)
	GetUnreadReportNotifications(page, limit int) ([]types.ReportNotification, *types.PaginationMetadata, error)
	MarkReportNotificationAsRead(id uint) error
	MarkAllReportNotificationsAsRead() error
	DeleteReportNotificationByEventID(eventID string) error

	// Report statistics
	GetReportStats() (*types.ReportStats, error)
	GetTotalReported() (int, error)
	GetTodayReportedCount() (int, error)
	GetReportsByType() ([]libtypes.TypeStat, error)
	GetMostReportedContent(limit int) ([]libtypes.ReportSummary, error)

	// NPUB access control management
	GetAllowedUser(npub string) (*types.AllowedUser, error)
	AddAllowedUser(npub string, tier string, createdBy string) error
	RemoveAllowedUser(npub string) error
	BulkAddAllowedUser(users []types.AllowedUser) error
	ClearAllowedUsers() error
	GetUsersPaginated(page int, pageSize int) ([]*types.AllowedUser, *types.PaginationMetadata, error)

	// Relay owner management
	GetRelayOwner() (*types.RelayOwner, error)
	SetRelayOwner(npub string, createdBy string) error
	RemoveRelayOwner() error

	// Bitcoin address management for mode switching
	GetAvailableBitcoinAddressCount() (int, error)
	CountUsersWithoutBitcoinAddresses() (int, error)

	// Push notification device management
	RegisterPushDevice(pubkey string, deviceToken string, platform string) error
	UnregisterPushDevice(pubkey string, deviceToken string) error
	GetPushDevicesByPubkey(pubkey string) ([]types.PushDevice, error)
	GetAllActivePushDevices() ([]types.PushDevice, error)
	UpdatePushDeviceStatus(deviceToken string, isActive bool) error
	CleanupInactivePushDevices(olderThan time.Time) error

	// Push notification logging
	LogPushNotification(log *types.PushNotificationLog) error
	GetPushNotificationHistory(pubkey string, limit int) ([]types.PushNotificationLog, error)
	UpdatePushNotificationDelivery(id uint, delivered bool, errorMessage string) error
}
