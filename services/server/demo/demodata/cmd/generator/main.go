package main

import (
	"bufio"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/badgerhold"
	"github.com/HORNET-Storage/hornet-storage/services/server/demo/demodata"
)

func main() {
	dbPath := "./data/store"

	dbPathPtr := flag.String("db", dbPath, "Path to SQLite database (same as used by the relay)")
	autoPtr := flag.Bool("auto", false, "Run with default settings without interaction")
	flag.Parse()

	// Initialize the SQLite store with the specified path

	store, err := badgerhold.InitStore(*dbPathPtr)
	if err != nil {
		logging.Fatalf("Failed to initialize BadgerHold store: %v", err)
	}

	defer store.Cleanup()

	// Create the generator
	generator := demodata.NewDemoDataGenerator()

	// If auto mode is enabled, generate data with default settings
	if *autoPtr {
		logging.Infof("Running in auto mode with default settings...")
		err := generateAllData(generator, store)
		if err != nil {
			logging.Infof("Error generating data: %v\n", err)
			os.Exit(1)
		}
		logging.Infof("Data generation complete!")
		logging.Infof("\n=======================================")
		return
	}

	// Otherwise, start the interactive CLI
	runInteractiveCLI(generator, store, *dbPathPtr)
}

// generateAllData generates all types of data
func generateAllData(generator *demodata.DemoDataGenerator, store *badgerhold.BadgerholdStore) error {
	return generator.GenerateAllData(store)
}

// runInteractiveCLI runs the interactive command-line interface
func runInteractiveCLI(generator *demodata.DemoDataGenerator, store *badgerhold.BadgerholdStore, dbPath string) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		printMainMenu()

		logging.Infof("> ")
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			err := generateAllData(generator, store)
			if err != nil {
				logging.Infof("Error generating all data: %v\n", err)
			} else {
				logging.Infof("\n=======================================")
				logging.Infof("Successfully generated data in:")
				logging.Infof("%s", dbPath)
				logging.Infof("=======================================")
			}

		case "2":
			err := generator.GenerateUserProfiles(store.StatsDatabase)
			if err != nil {
				logging.Infof("Error generating user profiles: %v\n", err)
			} else {
				logging.Infof("Successfully generated user profiles in: %s", dbPath)
			}

		case "3":
			err := generator.GenerateEventKinds(store.StatsDatabase)
			if err != nil {
				logging.Infof("Error generating event kinds: %v\n", err)
			} else {
				logging.Infof("Successfully generated event kinds in: %s", dbPath)
			}

		case "4":
			logging.Infof("How many payment notifications do you want to generate? [50]: ")
			scanner.Scan()
			countStr := strings.TrimSpace(scanner.Text())
			count := 50
			if countStr != "" {
				if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
					count = val
				}
			}

			err := generator.GeneratePaymentNotifications(store.StatsDatabase, count)
			if err != nil {
				logging.Infof("Error generating payment notifications: %v\n", err)
			} else {
				logging.Infof("Successfully generated %d payment notifications in: %s", count, dbPath)
			}

		case "5":
			err := generator.GenerateWalletBalance(store.StatsDatabase)
			if err != nil {
				logging.Infof("Error generating wallet balance history: %v\n", err)
			} else {
				logging.Infof("Successfully generated wallet balance history in: %s", dbPath)
			}

		case "6":
			logging.Infof("How many wallet transactions do you want to generate? [100]: ")
			scanner.Scan()
			countStr := strings.TrimSpace(scanner.Text())
			count := 100
			if countStr != "" {
				if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
					count = val
				}
			}

			err := generator.GenerateWalletTransactions(store.StatsDatabase, count)
			if err != nil {
				logging.Infof("Error generating wallet transactions: %v\n", err)
			} else {
				logging.Infof("Successfully generated %d wallet transactions in: %s", count, dbPath)
			}

		case "7":
			logging.Infof("How many wallet addresses do you want to generate? [50]: ")
			scanner.Scan()
			countStr := strings.TrimSpace(scanner.Text())
			count := 50
			if countStr != "" {
				if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
					count = val
				}
			}

			err := generator.GenerateWalletAddresses(store.StatsDatabase, count)
			if err != nil {
				logging.Infof("Error generating wallet addresses: %v\n", err)
			} else {
				logging.Infof("Successfully generated %d wallet addresses in: %s", count, dbPath)
			}

		case "8":
			configureTimeRange(scanner, generator)

		case "9":
			configureGrowthPatterns(scanner, generator)

		case "10":
			configureKindDistribution(scanner, generator)

		case "11":
			showCurrentSettings(generator)

		case "12":
			generator = demodata.NewDemoDataGenerator()
			logging.Infof("Settings reset to defaults.")

		case "0", "q", "quit", "exit":
			logging.Infof("\nExiting...")
			logging.Infof("Database location: %s\n", dbPath)
			return

		default:
			logging.Infof("Invalid choice. Please try again.")
		}

		logging.Infof("\nPress Enter to continue...")
		scanner.Scan()
	}
}

func printMainMenu() {
	logging.Infof("\nHORNETS-Nostr-Relay Demo Data Generator")
	logging.Infof("=======================================")
	logging.Infof("Main Menu:")
	logging.Infof("1. Generate All Demo Data (User Profiles, Events, Files, Wallet Data)")
	logging.Infof("2. Generate User Profiles Only (for Address Adoption charts)")
	logging.Infof("3. Generate Event Kinds Only (for Gigabytes Per Month charts)")
	logging.Infof("4. Generate Payment Notifications")
	logging.Infof("5. Generate Wallet Balance History")
	logging.Infof("6. Generate Wallet Transactions")
	logging.Infof("7. Generate Wallet Addresses")
	logging.Infof("8. Configure Time Range")
	logging.Infof("9. Configure Growth Patterns")
	logging.Infof("10. Configure Kind Distribution")
	logging.Infof("11. Show Current Settings")
	logging.Infof("12. Reset to Defaults")
	logging.Infof("0. Exit")
}

func configureTimeRange(scanner *bufio.Scanner, generator *demodata.DemoDataGenerator) {
	logging.Infof("\nConfigure Time Range")
	logging.Infof("===================")
	logging.Infof("Current range: %s to %s (%d months)\n\n",
		generator.StartMonth.Format("Jan 2006"),
		generator.EndMonth.Format("Jan 2006"),
		monthsBetween(generator.StartMonth, generator.EndMonth)+1)

	logging.Infof("1. Set start month (format: MM/YYYY)")
	logging.Infof("2. Set end month (format: MM/YYYY)")
	logging.Infof("3. Use last 12 months")
	logging.Infof("4. Use last 24 months")
	logging.Infof("5. Return to main menu")

	logging.Infof("\n> ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "1":
		logging.Infof("Enter start month (MM/YYYY) [%s]: ", generator.StartMonth.Format("01/2006"))
		scanner.Scan()
		dateStr := strings.TrimSpace(scanner.Text())
		if dateStr != "" {
			if date, err := time.Parse("01/2006", dateStr); err == nil {
				generator.StartMonth = date
				logging.Infof("Start month set to %s\n", date.Format("Jan 2006"))
			} else {
				logging.Infof("Invalid date format: %v\n", err)
			}
		}

	case "2":
		logging.Infof("Enter end month (MM/YYYY) [%s]: ", generator.EndMonth.Format("01/2006"))
		scanner.Scan()
		dateStr := strings.TrimSpace(scanner.Text())
		if dateStr != "" {
			if date, err := time.Parse("01/2006", dateStr); err == nil {
				generator.EndMonth = date
				logging.Infof("End month set to %s\n", date.Format("Jan 2006"))
			} else {
				logging.Infof("Invalid date format: %v\n", err)
			}
		}

	case "3":
		generator.EndMonth = time.Now()
		generator.StartMonth = generator.EndMonth.AddDate(-1, 0, 0)
		logging.Infof("Time range set to last 12 months: %s to %s\n",
			generator.StartMonth.Format("Jan 2006"),
			generator.EndMonth.Format("Jan 2006"))

	case "4":
		generator.EndMonth = time.Now()
		generator.StartMonth = generator.EndMonth.AddDate(-2, 0, 0)
		logging.Infof("Time range set to last 24 months: %s to %s\n",
			generator.StartMonth.Format("Jan 2006"),
			generator.EndMonth.Format("Jan 2006"))
	}
}

func configureGrowthPatterns(scanner *bufio.Scanner, generator *demodata.DemoDataGenerator) {
	logging.Infof("\nConfigure Growth Patterns")
	logging.Infof("=======================")
	logging.Infof("1. Set initial user count [current: %d]\n", generator.InitialUserCount)
	logging.Infof("2. Set monthly user growth rate [current: %.0f%%]\n", (generator.MonthlyUserGrowthRate-1.0)*100)
	logging.Infof("3. Set lightning address adoption rate [current: %.0f%%]\n", generator.LightningAdoptionRate*100)
	logging.Infof("4. Set DHT key adoption rate [current: %.0f%%]\n", generator.DHTKeyAdoptionRate*100)
	logging.Infof("5. Set both adoption rate [current: %.0f%%]\n", generator.BothAdoptionRate*100)
	logging.Infof("6. Set initial notes per day [current: %d]\n", generator.InitialNotesPerDay)
	logging.Infof("7. Set notes growth rate [current: %.0f%%]\n", (generator.NotesGrowthRate-1.0)*100)
	logging.Infof("8. Set media percentage [current: %.0f%%]\n", generator.MediaPercentage*100)
	logging.Infof("9. Return to main menu")

	logging.Infof("\n> ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "1":
		logging.Infof("Enter initial user count [%d]: ", generator.InitialUserCount)
		scanner.Scan()
		countStr := strings.TrimSpace(scanner.Text())
		if countStr != "" {
			if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
				generator.InitialUserCount = val
				logging.Infof("Initial user count set to %d\n", val)
			} else {
				logging.Infof("Invalid value: %v\n", err)
			}
		}

	case "2":
		logging.Infof("Enter monthly user growth rate in percent [%.0f]: ", (generator.MonthlyUserGrowthRate-1.0)*100)
		scanner.Scan()
		rateStr := strings.TrimSpace(scanner.Text())
		if rateStr != "" {
			if val, err := strconv.ParseFloat(rateStr, 64); err == nil && val >= 0 {
				generator.MonthlyUserGrowthRate = 1.0 + (val / 100.0)
				logging.Infof("Monthly user growth rate set to %.0f%% (factor: %.2f)\n", val, generator.MonthlyUserGrowthRate)
			} else {
				logging.Infof("Invalid value: %v\n", err)
			}
		}

	case "3":
		logging.Infof("Enter lightning address adoption rate in percent [%.0f]: ", generator.LightningAdoptionRate*100)
		scanner.Scan()
		rateStr := strings.TrimSpace(scanner.Text())
		if rateStr != "" {
			if val, err := strconv.ParseFloat(rateStr, 64); err == nil && val >= 0 && val <= 100 {
				generator.LightningAdoptionRate = val / 100.0
				logging.Infof("Lightning address adoption rate set to %.0f%%\n", val)
			} else {
				logging.Infof("Invalid value: %v\n", err)
			}
		}
	}
}

func configureKindDistribution(scanner *bufio.Scanner, generator *demodata.DemoDataGenerator) {
	logging.Infof("\nConfigure Kind Distribution")
	logging.Infof("==========================")
	logging.Infof("Current kind distribution:")
	for kind, percentage := range generator.KindDistribution {
		logging.Infof("- Kind %d: %.0f%%\n", kind, percentage*100)
	}

	logging.Infof("\n1. Modify kind percentages")
	logging.Infof("2. Return to main menu")

	logging.Infof("\n> ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	if choice == "1" {
		logging.Infof("\nEnter new percentages for each kind (must sum to 100%):")
		newDistribution := make(map[int]float64)
		totalPercentage := 0.0

		for kind := range generator.KindDistribution {
			logging.Infof("Kind %d [%.0f%%]: ", kind, generator.KindDistribution[kind]*100)
			scanner.Scan()
			percentStr := strings.TrimSpace(scanner.Text())

			if percentStr == "" {
				// Keep existing percentage
				newDistribution[kind] = generator.KindDistribution[kind]
				totalPercentage += generator.KindDistribution[kind]
			} else {
				if percent, err := strconv.ParseFloat(percentStr, 64); err == nil && percent >= 0 {
					newDistribution[kind] = percent / 100.0
					totalPercentage += percent / 100.0
				} else {
					logging.Infof("Invalid value: %v, using default\n", err)
					newDistribution[kind] = generator.KindDistribution[kind]
					totalPercentage += generator.KindDistribution[kind]
				}
			}
		}

		// Check if percentages sum to approximately 1.0 (allowing for small floating point errors)
		if totalPercentage > 0.99 && totalPercentage < 1.01 {
			generator.KindDistribution = newDistribution
			logging.Infof("Kind distribution updated successfully")
		} else {
			logging.Infof("Error: Percentages sum to %.2f%%, must be 100%%\n", totalPercentage*100)
		}
	}
}

func showCurrentSettings(generator *demodata.DemoDataGenerator) {
	logging.Infof("\nCurrent Settings")
	logging.Infof("===============")
	logging.Infof("Time Range: %s to %s (%d months)\n",
		generator.StartMonth.Format("Jan 2006"),
		generator.EndMonth.Format("Jan 2006"),
		monthsBetween(generator.StartMonth, generator.EndMonth)+1)

	logging.Infof("Initial User Count: %d\n", generator.InitialUserCount)
	logging.Infof("Monthly User Growth Rate: %.0f%% (factor: %.2f)\n",
		(generator.MonthlyUserGrowthRate-1.0)*100, generator.MonthlyUserGrowthRate)
	logging.Infof("Lightning Address Adoption: %.0f%%\n", generator.LightningAdoptionRate*100)
	logging.Infof("DHT Key Adoption: %.0f%%\n", generator.DHTKeyAdoptionRate*100)
	logging.Infof("Both Adoption: %.0f%%\n", generator.BothAdoptionRate*100)

	logging.Infof("Initial Notes Per Day: %d\n", generator.InitialNotesPerDay)
	logging.Infof("Notes Growth Rate: %.0f%% (factor: %.2f)\n",
		(generator.NotesGrowthRate-1.0)*100, generator.NotesGrowthRate)
	logging.Infof("Media Percentage: %.0f%%\n", generator.MediaPercentage*100)

	logging.Infof("\nKind Distribution:")
	for kind, percentage := range generator.KindDistribution {
		logging.Infof("- Kind %d: %.0f%%\n", kind, percentage*100)
	}
}

// findProjectRoot attempts to locate the project root directory by looking for go.mod
func findProjectRoot() (string, error) {
	// Start with the current directory
	currentDir, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to get current directory: %v", err)
	}

	// Traverse up the directory tree looking for go.mod
	for {
		// Check if go.mod exists in the current directory
		goModPath := filepath.Join(currentDir, "go.mod")
		if _, err := os.Stat(goModPath); err == nil {
			// Found go.mod, this is likely the project root
			return currentDir, nil
		}

		// Move up one directory
		parentDir := filepath.Dir(currentDir)
		if parentDir == currentDir {
			// We've reached the filesystem root without finding go.mod
			return "", fmt.Errorf("project root not found: go.mod not found in any parent directory")
		}
		currentDir = parentDir
	}
}

// Helper function to calculate months between two dates
func monthsBetween(start, end time.Time) int {
	months := 0
	current := start

	for !current.After(end) {
		months++
		current = current.AddDate(0, 1, 0)
	}

	return months - 1 // Subtract 1 because we want the range, not the count
}
