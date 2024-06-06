package graviton

import (
	"fmt"
	"log"
	"sync"

	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	jsoniter "github.com/json-iterator/go"
)

var (
	once     sync.Once
	instance *gorm.DB
	err      error
)

// InitGorm initializes the GORM DB (This will handle the SQLite DB for Relay Stats)
func InitGorm() (*gorm.DB, error) {
	once.Do(func() {
		instance, err = gorm.Open(sqlite.Open("relay_stats.db"), &gorm.Config{})
		if err != nil {
			log.Fatalf("Failed to connect to database: %v", err)
		}

		// Auto migrate the schema
		err = instance.AutoMigrate(
			&types.Kind{},
			&types.Photo{},
			&types.Video{},
			&types.GitNestr{},
			&types.UserProfile{},
			&types.User{},
			&types.WalletBalance{},
			&types.WalletTransactions{},
			&types.BitcoinRate{},
			&types.WalletAddress{},
			&types.UserChallenge{},
		)
		if err != nil {
			log.Fatalf("Failed to migrate database schema: %v", err)
		}
	})
	return instance, err
}

func storeInGorm(event *nostr.Event) {
	gormDB, err := InitGorm()
	if err != nil {
		log.Printf("Error initializing GORM: %v", err)
		return
	}

	kindStr := fmt.Sprintf("kind%d", event.Kind)

	var relaySettings types.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
		log.Fatalf("Error unmarshaling relay settings: %v", err)
	}

	if event.Kind == 0 {
		// Handle user profile creation or update
		var contentData map[string]interface{}
		if err := jsoniter.Unmarshal([]byte(event.Content), &contentData); err != nil {
			log.Printf("Error unmarshaling event content: %v", err)
			return
		}

		npubKey := event.PubKey
		lightningAddr := false
		dhtKey := false

		if nip05, ok := contentData["nip05"].(string); ok && nip05 != "" {
			lightningAddr = true
		}

		if dht, ok := contentData["dht-key"].(string); ok && dht != "" {
			dhtKey = true
		}

		err := upsertUserProfile(gormDB, npubKey, lightningAddr, dhtKey)
		if err != nil {
			log.Printf("Error upserting user profile: %v", err)
		}
	}

	if contains(relaySettings.Kinds, kindStr) {
		// Calculate size of the event in bytes
		sizeBytes := len(event.ID) + len(event.PubKey) + len(event.Content) + len(event.Sig)
		for _, tag := range event.Tags {
			for _, t := range tag {
				sizeBytes += len(t)
			}
		}
		sizeMB := float64(sizeBytes) / (1024 * 1024) // Convert to MB

		kind := types.Kind{
			KindNumber: event.Kind,
			EventID:    event.ID,
			Size:       sizeMB,
		}
		gormDB.Create(&kind)
		return
	}

	// Add cases for photos, videos, and gitNestr
	fmt.Printf("Unhandled kind: %d\n", event.Kind)
}

func upsertUserProfile(db *gorm.DB, npubKey string, lightningAddr, dhtKey bool) error {
	var userProfile types.UserProfile
	result := db.Where("npub_key = ?", npubKey).First(&userProfile)

	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			// Create new user profile
			userProfile = types.UserProfile{
				NpubKey:       npubKey,
				LightningAddr: lightningAddr,
				DHTKey:        dhtKey,
			}
			return db.Create(&userProfile).Error
		}
		return result.Error
	}

	// Update existing user profile
	userProfile.LightningAddr = lightningAddr
	userProfile.DHTKey = dhtKey
	return db.Save(&userProfile).Error
}

func contains(slice []string, item string) bool {
	for _, v := range slice {
		if v == item {
			return true
		}
	}
	return false
}
