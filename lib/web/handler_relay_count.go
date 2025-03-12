package web

import (
	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Refactored getRelayCount function
func getRelayCount(c *fiber.Ctx, store stores.Store) error {
	// Initialize the response data
	responseData := map[string]int{}

	relaySettings, err := RetrieveSettings()
	if err != nil {
		return err
	}

	// Fetch counts from the statistics store
	for group, types := range relaySettings.MimeTypeGroups {
		for _, mimeType := range types {
			amount, err := store.GetStatsStore().FetchFileCountByType(mimeType)
			if err != nil {
				continue // Skip if there's an error fetching count
			}

			if _, ok := responseData[group]; !ok {
				responseData[group] = 0
			}

			responseData[group] += amount
		}
	}

	// Add notes count (Nostr events) to the response
	noteCount, err := store.GetStatsStore().FetchKindCount()
	if err == nil { // Only add if there was no error
		responseData["Notes"] = noteCount
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
