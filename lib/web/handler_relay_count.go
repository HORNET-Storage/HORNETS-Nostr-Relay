package web

import (
	"log"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
	"github.com/spf13/viper"
)

// Refactored getRelayCount function
func getRelayCount(c *fiber.Ctx, store stores.Store) error {
	log.Println("Relay count request received")

	// Retrieve relay settings from the config file using Viper
	var relaySettings struct {
		GitNestr []string `mapstructure:"gitNestr"`
	}
	if err := viper.UnmarshalKey("relay_settings", &relaySettings); err != nil {
		log.Fatalf("Error unmarshaling relay settings: %v", err)
	}

	// Initialize the response data
	responseData := map[string]int{
		"kinds":    0,
		"photos":   0,
		"videos":   0,
		"gitNestr": 0,
		"audio":    0,
		"misc":     0,
	}

	// Fetch counts from the statistics store
	var err error
	responseData["kinds"], err = store.GetStatsStore().FetchKindCount()
	if err != nil {
		log.Printf("Error getting kind counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting kind counts")
	}

	responseData["photos"], err = store.GetStatsStore().FetchPhotoCount()
	if err != nil {
		log.Printf("Error getting photo counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting photo counts")
	}

	responseData["videos"], err = store.GetStatsStore().FetchVideoCount()
	if err != nil {
		log.Printf("Error getting video counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting video counts")
	}

	responseData["gitNestr"], err = store.GetStatsStore().FetchGitNestrCount(relaySettings.GitNestr)
	if err != nil {
		log.Printf("Error getting gitNestr counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting gitNestr counts")
	}

	responseData["audio"], err = store.GetStatsStore().FetchAudioCount()
	if err != nil {
		log.Printf("Error getting audio counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting audio counts")
	}

	responseData["misc"], err = store.GetStatsStore().FetchMiscCount()
	if err != nil {
		log.Printf("Error getting misc counts: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Error getting misc counts")
	}

	// Respond with the aggregated counts
	return c.JSON(responseData)
}
