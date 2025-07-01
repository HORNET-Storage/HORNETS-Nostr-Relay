package demodata

import (
	"fmt"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
)

// GenerateUserProfiles creates user profiles with adoption metrics for each month
func (g *DemoDataGenerator) GenerateUserProfiles(store statistics.StatisticsStore) error {
	fmt.Println("Generating user profiles month by month...")

	// Start with base number of users
	currentUsers := g.InitialUserCount

	// Generate data for each month in the range
	currentMonth := g.StartMonth
	for !currentMonth.After(g.EndMonth) {
		fmt.Printf("Generating profiles for %s...\n", currentMonth.Format("Jan 2006"))

		// Calculate how many users to add this month
		usersThisMonth := currentUsers

		// We need to ensure each profile has a unique date within the month
		// so they don't get grouped together in the SQL query

		// Get the number of days in this month to distribute profiles
		daysInCurrentMonth := daysInMonth(currentMonth)

		// Distribute profiles across the days of the month
		profilesPerDay := make(map[int]int)

		// Distribute profiles evenly across days first
		baseProfilesPerDay := usersThisMonth / daysInCurrentMonth
		remainingProfiles := usersThisMonth % daysInCurrentMonth

		// Initialize profiles per day
		for day := 1; day <= daysInCurrentMonth; day++ {
			profilesPerDay[day] = baseProfilesPerDay
			if remainingProfiles > 0 {
				profilesPerDay[day]++
				remainingProfiles--
			}
		}

		// Now generate profiles for each day
		for day := 1; day <= daysInCurrentMonth; day++ {
			numProfiles := profilesPerDay[day]

			// Create a specific date for this day in the month
			specificDate := time.Date(currentMonth.Year(), currentMonth.Month(), day, 0, 0, 0, 0, currentMonth.Location())

			// Create profiles for this day
			for i := 0; i < numProfiles; i++ {
				// Determine profile characteristics
				hasLightning := g.rng.Float64() < g.LightningAdoptionRate
				hasDHT := g.rng.Float64() < g.DHTKeyAdoptionRate

				// If both adoption is specified, override individual settings sometimes
				if g.rng.Float64() < g.BothAdoptionRate {
					hasLightning = true
					hasDHT = true
				}

				// Create a unique pubkey
				pubkey := g.generatePubKey()

				// Create a random time within this specific day
				hour := g.rng.Intn(24)
				minute := g.rng.Intn(60)
				second := g.rng.Intn(60)
				timestamp := time.Date(specificDate.Year(), specificDate.Month(), specificDate.Day(),
					hour, minute, second, 0, specificDate.Location())

				// Use the store's UpsertUserProfile method
				if err := store.UpsertUserProfile(
					pubkey,
					hasLightning,
					hasDHT,
					timestamp,
				); err != nil {
					return fmt.Errorf("error generating profile: %v", err)
				}
			}
		}

		// Increase users for next month based on growth rate
		currentUsers = int(float64(currentUsers) * g.MonthlyUserGrowthRate)

		// Move to next month
		currentMonth = addMonth(currentMonth)
	}

	fmt.Println("User profile generation complete!")
	return nil
}
