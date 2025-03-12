package web

import (
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Refactored getRelayCount function
func getRelayCount(c *fiber.Ctx, store stores.Store) error {
	// Initialize the response data with the keys expected by the frontend
	responseData := map[string]int{
		"kinds":  0,
		"photos": 0,
		"videos": 0,
		"audio":  0,
		"misc":   0,
	}

	relaySettings, err := RetrieveSettings()
	if err != nil {
		return err
	}

	// Map config group names to frontend expected names
	groupMapping := map[string]string{
		"images":    "photos",
		"videos":    "videos",
		"audio":     "audio",
		"documents": "misc",
	}

	// Fetch counts from the statistics store
	for group, types := range relaySettings.MimeTypeGroups {
		frontendGroup := groupMapping[group]
		if frontendGroup == "" {
			// If no mapping exists, use the original group name
			frontendGroup = group
		}

		for _, mimeType := range types {
			amount, err := store.GetStatsStore().FetchFileCountByType(mimeType)
			if err != nil {
				continue // Skip if there's an error fetching count
			}

			responseData[frontendGroup] += amount
		}
	}

	// Add notes count (Nostr events) to the response
	noteCount, err := store.GetStatsStore().FetchKindCount()
	if err == nil { // Only add if there was no error
		responseData["kinds"] = noteCount
	}

	// Respond with the aggregated counts
	return c.JSON(responseData)
}

func RetrieveSettings() (*types.RelaySettings, error) {
	var settings types.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

// func contains(list []string, item string) bool {
// 	for _, element := range list {
// 		if element == item {
// 			return true
// 		}
// 	}
// 	return false
// }
