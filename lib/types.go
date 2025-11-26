package lib

import "github.com/HORNET-Storage/hornet-storage/lib/types"

// This file re-exports types from the hornet-storage library for backwards compatibility and will eventually be removed.

// Storage types
type (
	LeafContent         = types.LeafContent
	DagOwnership        = types.DagOwnership
	WrappedLeaf         = types.WrappedLeaf
	AdditionalDataEntry = types.AdditionalDataEntry
	DagLeafData         = types.DagLeafData
	DagData             = types.DagData
	UploadMessage       = types.UploadMessage
	DownloadMessage     = types.DownloadMessage
	LeafLabelRange      = types.LeafLabelRange
	DownloadFilter      = types.DownloadFilter
	BlockData           = types.BlockData
	FileInfo            = types.FileInfo
	FileTag             = types.FileTag
	DagContent          = types.DagContent
	BlobContent         = types.BlobContent
	BlobDescriptor      = types.BlobDescriptor
	CacheMetaData       = types.CacheMetaData
	CacheData           = types.CacheData
)

// Nostr types
type (
	NostrEvent           = types.NostrEvent
	TagEntry             = types.TagEntry
	Kind                 = types.Kind
	QueryFilter          = types.QueryFilter
	QueryMessage         = types.QueryMessage
	AdvancedQueryMessage = types.AdvancedQueryMessage
	QueryResponse        = types.QueryResponse
	ResponseMessage      = types.ResponseMessage
	ErrorMessage         = types.ErrorMessage
)

// Moderation types
type (
	PendingModeration        = types.PendingModeration
	BlockedEvent             = types.BlockedEvent
	PendingDisputeModeration = types.PendingDisputeModeration
	BlockedPubkey            = types.BlockedPubkey
	ModerationNotification   = types.ModerationNotification
	ModerationStats          = types.ModerationStats
	ReportNotification       = types.ReportNotification
	ReportStats              = types.ReportStats
	ReportSummary            = types.ReportSummary
)

// Auth types
type (
	UserProfile   = types.UserProfile
	ActiveToken   = types.ActiveToken
	AdminUser     = types.AdminUser
	UserChallenge = types.UserChallenge
	LoginRequest  = types.LoginRequest
	SignUpRequest = types.SignUpRequest
	LoginPayload  = types.LoginPayload
	JWTClaims     = types.JWTClaims
)

// Payment types
type (
	SubscriptionTier    = types.SubscriptionTier
	Subscriber          = types.Subscriber
	SubscriberAddress   = types.SubscriberAddress
	PaidSubscriber      = types.PaidSubscriber
	PaymentNotification = types.PaymentNotification
	PaymentStats        = types.PaymentStats
	TierStat            = types.TierStat
	TxSummary           = types.TxSummary
)

// Analytics types
type (
	PaginationMetadata = types.PaginationMetadata
	TimeSeriesData     = types.TimeSeriesData
	ActivityData       = types.ActivityData
	BarChartData       = types.BarChartData
	AggregatedKindData = types.AggregatedKindData
	KindData           = types.KindData
	MonthlyKindData    = types.MonthlyKindData
	TypeStat           = types.TypeStat
	UserStat           = types.UserStat
)

// Network types
type (
	Stream          = types.Stream
	WebSocketStream = types.WebSocketStream
)

// Wallet types
type (
	WalletBalance             = types.WalletBalance
	WalletTransactions        = types.WalletTransactions
	WalletAddress             = types.WalletAddress
	BitcoinRate               = types.BitcoinRate
	PendingTransaction        = types.PendingTransaction
	ReplaceTransactionRequest = types.ReplaceTransactionRequest
	Address                   = types.Address
	AddressResponse           = types.AddressResponse
)

// Access types
type (
	AllowedUser = types.AllowedUser
	RelayOwner  = types.RelayOwner
)

// Config types
type (
	Config                  = types.Config
	ServerConfig            = types.ServerConfig
	ExternalServicesConfig  = types.ExternalServicesConfig
	OllamaConfig            = types.OllamaConfig
	ModeratorConfig         = types.ModeratorConfig
	WalletConfig            = types.WalletConfig
	LoggingConfig           = types.LoggingConfig
	RelayConfig             = types.RelayConfig
	ContentFilteringConfig  = types.ContentFilteringConfig
	TextFilterConfig        = types.TextFilterConfig
	ImageModerationConfig   = types.ImageModerationConfig
	EventFilteringConfig    = types.EventFilteringConfig
	DynamicKindsConfig      = types.DynamicKindsConfig
	ProtocolsConfig         = types.ProtocolsConfig
	SubscriptionTiersConfig = types.SubscriptionTiers
	FreeTierConfig          = types.SubscriptionTier
)

// Re-export constructor functions
var (
	NewWebSocketStream = types.NewWebSocketStream
)
