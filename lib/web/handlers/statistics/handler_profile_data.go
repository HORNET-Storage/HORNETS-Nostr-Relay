package statistics

import (
	"log"
	"time"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Refactored getProfilesTimeSeriesData function
func GetProfilesTimeSeriesData(c *fiber.Ctx, store stores.Store) error {
	log.Println("Time series request received")

	// Calculate the date range for the last 6 months, making sure to include full current month
	now := time.Now()
	// End date should be formatted to include the entire current month
	// We'll add a day to the end of the month to make sure we include all records
	endDate := time.Date(now.Year(), now.Month()+1, 1, 0, 0, 0, 0, now.Location())
	// Start date is 6 months before current month
	startDate := time.Date(now.Year(), now.Month()-5, 1, 0, 0, 0, 0, now.Location())

	log.Printf("Start date: %s, End date: %s",
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))

	// Fetch the time series data from the statistics store
	data, err := store.GetStatsStore().FetchProfilesTimeSeriesData(
		startDate.Format("2006-01-02"),
		endDate.Format("2006-01-02"))
	if err != nil {
		log.Println("Error fetching time series data:", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	// Create a complete list of the last 6 months
	completeData := make([]types.TimeSeriesData, 6)

	// Start from the current month and go back 6 months
	currentMonth := now

	for i := 0; i < 6; i++ {
		// For each iteration, we want the 1st day of the month
		month := time.Date(currentMonth.Year(), currentMonth.Month()-time.Month(i), 1, 0, 0, 0, 0, currentMonth.Location())
		formattedMonth := month.Format("2006-01")
		completeData[5-i] = types.TimeSeriesData{Month: formattedMonth}
	}

	log.Printf("Complete list of months: %+v", completeData)

	// Merge queried data with the complete list
	dataMap := make(map[string]types.TimeSeriesData)
	for _, d := range data {
		dataMap[d.Month] = d
	}

	for i, cd := range completeData {
		if d, exists := dataMap[cd.Month]; exists {
			completeData[i] = d
		}
	}

	log.Printf("Fetched data for the last 6 months: %+v", completeData)
	return c.JSON(completeData)
}
