package settings

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// GetRelayCount function - simplified version
func GetRelayCount(c *fiber.Ctx, store stores.Store) error {
	// Initialize the response data
	responseData := map[string]int{
		"kinds":  0,
		"photos": 0,
		"videos": 0,
		"audio":  0,
		"misc":   0,
	}

	// Get media type extensions directly from config
	photoExtensions := viper.GetStringSlice("event_filtering.media_types.photos.allowed_extensions")
	videoExtensions := viper.GetStringSlice("event_filtering.media_types.videos.allowed_extensions")
	audioExtensions := viper.GetStringSlice("event_filtering.media_types.audio.allowed_extensions")
	miscExtensions := viper.GetStringSlice("event_filtering.media_types.git_repositories.allowed_patterns")

	// Count photos
	for _, ext := range photoExtensions {
		if count, err := store.GetStatsStore().FetchFileCountByType(ext); err == nil {
			responseData["photos"] += count
		}
	}

	// Count videos
	for _, ext := range videoExtensions {
		if count, err := store.GetStatsStore().FetchFileCountByType(ext); err == nil {
			responseData["videos"] += count
		}
	}

	// Count audio
	for _, ext := range audioExtensions {
		if count, err := store.GetStatsStore().FetchFileCountByType(ext); err == nil {
			responseData["audio"] += count
		}
	}

	// Count misc/documents
	for _, ext := range miscExtensions {
		if count, err := store.GetStatsStore().FetchFileCountByType(ext); err == nil {
			responseData["misc"] += count
		}
	}

	// Add notes count (Nostr events)
	if noteCount, err := store.GetStatsStore().FetchKindCount(); err == nil {
		responseData["kinds"] = noteCount
	}

	return c.JSON(responseData)
}
