package settings

import (
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// GetRelayCount function - dynamic version using media_definitions
func GetRelayCount(c *fiber.Ctx, store stores.Store) error {
	// Initialize response data with kinds count
	responseData := make(map[string]int)

	// Add Nostr events count
	if kindCount, err := store.GetStatsStore().FetchKindCount(); err == nil {
		responseData["kinds"] = kindCount
	} else {
		responseData["kinds"] = 0
	}

	// Get media definitions from config dynamically
	mediaDefinitions := make(map[string]types.MediaDefinition)
	if err := viper.UnmarshalKey("event_filtering.media_definitions", &mediaDefinitions); err != nil {
		// If config loading fails, return basic response with just kinds
		return c.JSON(responseData)
	}

	// Dynamically count files for each media type defined in config
	for mediaType, definition := range mediaDefinitions {
		totalCount := 0
		
		// Count files by extensions for this media type
		for _, ext := range definition.Extensions {
			if count, err := store.GetStatsStore().FetchFileCountByType(ext); err == nil {
				totalCount += count
			}
		}
		
		// Add to response with the media type name as key
		responseData[mediaType] = totalCount
	}

	return c.JSON(responseData)
}
