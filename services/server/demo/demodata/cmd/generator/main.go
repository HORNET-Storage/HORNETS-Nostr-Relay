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

	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm/sqlite_demo"
	"github.com/HORNET-Storage/hornet-storage/services/server/demo/demodata"
)

func main() {
	// Find project root and set default DB path to project root
	projectRoot, err := findProjectRoot()
	if err != nil {
		fmt.Printf("Warning: Could not determine project root: %v\n", err)
		fmt.Println("Using current directory as a fallback.")
		projectRoot, _ = os.Getwd()
	}
	defaultDBPath := filepath.Join(projectRoot, "demo_statistics.db")

	// Parse command line flags
	dbPathPtr := flag.String("db", defaultDBPath, "Path to SQLite database (same as used by the demo server)")
	autoPtr := flag.Bool("auto", false, "Run with default settings without interaction")
	flag.Parse()

	dbPath := *dbPathPtr

	fmt.Println("HORNETS-Nostr-Relay Demo Data Generator")
	fmt.Println("=======================================")
	fmt.Printf("Using database: %s\n\n", dbPath)

	// Initialize the SQLite demo store
	store, err := sqlite_demo.InitStore(dbPath)
	if err != nil {
		fmt.Printf("Error initializing SQLite demo store: %v\n", err)
		os.Exit(1)
	}

	// Create the generator
	generator := demodata.NewDemoDataGenerator()

	// If auto mode is enabled, generate data with default settings
	if *autoPtr {
		fmt.Println("Running in auto mode with default settings...")
		err := generateAllData(generator, store)
		if err != nil {
			fmt.Printf("Error generating data: %v\n", err)
			os.Exit(1)
		}
		fmt.Println("Data generation complete!")
		return
	}

	// Otherwise, start the interactive CLI
	runInteractiveCLI(generator, store)
}

// generateAllData generates all types of data
func generateAllData(generator *demodata.DemoDataGenerator, store statistics.StatisticsStore) error {
	return generator.GenerateAllData(store)
}

// runInteractiveCLI runs the interactive command-line interface
func runInteractiveCLI(generator *demodata.DemoDataGenerator, store statistics.StatisticsStore) {
	scanner := bufio.NewScanner(os.Stdin)

	for {
		printMainMenu()

		fmt.Print("> ")
		scanner.Scan()
		choice := strings.TrimSpace(scanner.Text())

		switch choice {
		case "1":
			err := generateAllData(generator, store)
			if err != nil {
				fmt.Printf("Error generating all data: %v\n", err)
			}

		case "2":
			err := generator.GenerateUserProfiles(store)
			if err != nil {
				fmt.Printf("Error generating user profiles: %v\n", err)
			}

		case "3":
			err := generator.GenerateEventKinds(store)
			if err != nil {
				fmt.Printf("Error generating event kinds: %v\n", err)
			}

		case "4":
			fmt.Print("How many payment notifications do you want to generate? [50]: ")
			scanner.Scan()
			countStr := strings.TrimSpace(scanner.Text())
			count := 50
			if countStr != "" {
				if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
					count = val
				}
			}

			err := generator.GeneratePaymentNotifications(store, count)
			if err != nil {
				fmt.Printf("Error generating payment notifications: %v\n", err)
			}

		case "5":
			err := generator.GenerateWalletBalance(store)
			if err != nil {
				fmt.Printf("Error generating wallet balance history: %v\n", err)
			}

		case "6":
			fmt.Print("How many wallet transactions do you want to generate? [100]: ")
			scanner.Scan()
			countStr := strings.TrimSpace(scanner.Text())
			count := 100
			if countStr != "" {
				if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
					count = val
				}
			}

			err := generator.GenerateWalletTransactions(store, count)
			if err != nil {
				fmt.Printf("Error generating wallet transactions: %v\n", err)
			}

		case "7":
			fmt.Print("How many wallet addresses do you want to generate? [50]: ")
			scanner.Scan()
			countStr := strings.TrimSpace(scanner.Text())
			count := 50
			if countStr != "" {
				if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
					count = val
				}
			}

			err := generator.GenerateWalletAddresses(store, count)
			if err != nil {
				fmt.Printf("Error generating wallet addresses: %v\n", err)
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
			fmt.Println("Settings reset to defaults.")

		case "0", "q", "quit", "exit":
			fmt.Println("Exiting...")
			return

		default:
			fmt.Println("Invalid choice. Please try again.")
		}

		fmt.Println("\nPress Enter to continue...")
		scanner.Scan()
	}
}

func printMainMenu() {
	fmt.Println("\nHORNETS-Nostr-Relay Demo Data Generator")
	fmt.Println("=======================================")
	fmt.Println("Main Menu:")
	fmt.Println("1. Generate All Demo Data (User Profiles, Events, Files, Wallet Data)")
	fmt.Println("2. Generate User Profiles Only (for Address Adoption charts)")
	fmt.Println("3. Generate Event Kinds Only (for Gigabytes Per Month charts)")
	fmt.Println("4. Generate Payment Notifications")
	fmt.Println("5. Generate Wallet Balance History")
	fmt.Println("6. Generate Wallet Transactions")
	fmt.Println("7. Generate Wallet Addresses")
	fmt.Println("8. Configure Time Range")
	fmt.Println("9. Configure Growth Patterns")
	fmt.Println("10. Configure Kind Distribution")
	fmt.Println("11. Show Current Settings")
	fmt.Println("12. Reset to Defaults")
	fmt.Println("0. Exit")
}

func configureTimeRange(scanner *bufio.Scanner, generator *demodata.DemoDataGenerator) {
	fmt.Println("\nConfigure Time Range")
	fmt.Println("===================")
	fmt.Printf("Current range: %s to %s (%d months)\n\n",
		generator.StartMonth.Format("Jan 2006"),
		generator.EndMonth.Format("Jan 2006"),
		monthsBetween(generator.StartMonth, generator.EndMonth)+1)

	fmt.Println("1. Set start month (format: MM/YYYY)")
	fmt.Println("2. Set end month (format: MM/YYYY)")
	fmt.Println("3. Use last 12 months")
	fmt.Println("4. Use last 24 months")
	fmt.Println("5. Return to main menu")

	fmt.Print("\n> ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "1":
		fmt.Printf("Enter start month (MM/YYYY) [%s]: ", generator.StartMonth.Format("01/2006"))
		scanner.Scan()
		dateStr := strings.TrimSpace(scanner.Text())
		if dateStr != "" {
			if date, err := time.Parse("01/2006", dateStr); err == nil {
				generator.StartMonth = date
				fmt.Printf("Start month set to %s\n", date.Format("Jan 2006"))
			} else {
				fmt.Printf("Invalid date format: %v\n", err)
			}
		}

	case "2":
		fmt.Printf("Enter end month (MM/YYYY) [%s]: ", generator.EndMonth.Format("01/2006"))
		scanner.Scan()
		dateStr := strings.TrimSpace(scanner.Text())
		if dateStr != "" {
			if date, err := time.Parse("01/2006", dateStr); err == nil {
				generator.EndMonth = date
				fmt.Printf("End month set to %s\n", date.Format("Jan 2006"))
			} else {
				fmt.Printf("Invalid date format: %v\n", err)
			}
		}

	case "3":
		generator.EndMonth = time.Now()
		generator.StartMonth = generator.EndMonth.AddDate(-1, 0, 0)
		fmt.Printf("Time range set to last 12 months: %s to %s\n",
			generator.StartMonth.Format("Jan 2006"),
			generator.EndMonth.Format("Jan 2006"))

	case "4":
		generator.EndMonth = time.Now()
		generator.StartMonth = generator.EndMonth.AddDate(-2, 0, 0)
		fmt.Printf("Time range set to last 24 months: %s to %s\n",
			generator.StartMonth.Format("Jan 2006"),
			generator.EndMonth.Format("Jan 2006"))
	}
}

func configureGrowthPatterns(scanner *bufio.Scanner, generator *demodata.DemoDataGenerator) {
	fmt.Println("\nConfigure Growth Patterns")
	fmt.Println("=======================")
	fmt.Printf("1. Set initial user count [current: %d]\n", generator.InitialUserCount)
	fmt.Printf("2. Set monthly user growth rate [current: %.0f%%]\n", (generator.MonthlyUserGrowthRate-1.0)*100)
	fmt.Printf("3. Set lightning address adoption rate [current: %.0f%%]\n", generator.LightningAdoptionRate*100)
	fmt.Printf("4. Set DHT key adoption rate [current: %.0f%%]\n", generator.DHTKeyAdoptionRate*100)
	fmt.Printf("5. Set both adoption rate [current: %.0f%%]\n", generator.BothAdoptionRate*100)
	fmt.Printf("6. Set initial notes per day [current: %d]\n", generator.InitialNotesPerDay)
	fmt.Printf("7. Set notes growth rate [current: %.0f%%]\n", (generator.NotesGrowthRate-1.0)*100)
	fmt.Printf("8. Set media percentage [current: %.0f%%]\n", generator.MediaPercentage*100)
	fmt.Println("9. Return to main menu")

	fmt.Print("\n> ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	switch choice {
	case "1":
		fmt.Printf("Enter initial user count [%d]: ", generator.InitialUserCount)
		scanner.Scan()
		countStr := strings.TrimSpace(scanner.Text())
		if countStr != "" {
			if val, err := strconv.Atoi(countStr); err == nil && val > 0 {
				generator.InitialUserCount = val
				fmt.Printf("Initial user count set to %d\n", val)
			} else {
				fmt.Printf("Invalid value: %v\n", err)
			}
		}

	case "2":
		fmt.Printf("Enter monthly user growth rate in percent [%.0f]: ", (generator.MonthlyUserGrowthRate-1.0)*100)
		scanner.Scan()
		rateStr := strings.TrimSpace(scanner.Text())
		if rateStr != "" {
			if val, err := strconv.ParseFloat(rateStr, 64); err == nil && val >= 0 {
				generator.MonthlyUserGrowthRate = 1.0 + (val / 100.0)
				fmt.Printf("Monthly user growth rate set to %.0f%% (factor: %.2f)\n", val, generator.MonthlyUserGrowthRate)
			} else {
				fmt.Printf("Invalid value: %v\n", err)
			}
		}

	case "3":
		fmt.Printf("Enter lightning address adoption rate in percent [%.0f]: ", generator.LightningAdoptionRate*100)
		scanner.Scan()
		rateStr := strings.TrimSpace(scanner.Text())
		if rateStr != "" {
			if val, err := strconv.ParseFloat(rateStr, 64); err == nil && val >= 0 && val <= 100 {
				generator.LightningAdoptionRate = val / 100.0
				fmt.Printf("Lightning address adoption rate set to %.0f%%\n", val)
			} else {
				fmt.Printf("Invalid value: %v\n", err)
			}
		}
	}
}

func configureKindDistribution(scanner *bufio.Scanner, generator *demodata.DemoDataGenerator) {
	fmt.Println("\nConfigure Kind Distribution")
	fmt.Println("==========================")
	fmt.Println("Current kind distribution:")
	for kind, percentage := range generator.KindDistribution {
		fmt.Printf("- Kind %d: %.0f%%\n", kind, percentage*100)
	}

	fmt.Println("\n1. Modify kind percentages")
	fmt.Println("2. Return to main menu")

	fmt.Print("\n> ")
	scanner.Scan()
	choice := strings.TrimSpace(scanner.Text())

	if choice == "1" {
		fmt.Println("\nEnter new percentages for each kind (must sum to 100%):")
		newDistribution := make(map[int]float64)
		totalPercentage := 0.0

		for kind := range generator.KindDistribution {
			fmt.Printf("Kind %d [%.0f%%]: ", kind, generator.KindDistribution[kind]*100)
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
					fmt.Printf("Invalid value: %v, using default\n", err)
					newDistribution[kind] = generator.KindDistribution[kind]
					totalPercentage += generator.KindDistribution[kind]
				}
			}
		}

		// Check if percentages sum to approximately 1.0 (allowing for small floating point errors)
		if totalPercentage > 0.99 && totalPercentage < 1.01 {
			generator.KindDistribution = newDistribution
			fmt.Println("Kind distribution updated successfully")
		} else {
			fmt.Printf("Error: Percentages sum to %.2f%%, must be 100%%\n", totalPercentage*100)
		}
	}
}

func showCurrentSettings(generator *demodata.DemoDataGenerator) {
	fmt.Println("\nCurrent Settings")
	fmt.Println("===============")
	fmt.Printf("Time Range: %s to %s (%d months)\n",
		generator.StartMonth.Format("Jan 2006"),
		generator.EndMonth.Format("Jan 2006"),
		monthsBetween(generator.StartMonth, generator.EndMonth)+1)

	fmt.Printf("Initial User Count: %d\n", generator.InitialUserCount)
	fmt.Printf("Monthly User Growth Rate: %.0f%% (factor: %.2f)\n",
		(generator.MonthlyUserGrowthRate-1.0)*100, generator.MonthlyUserGrowthRate)
	fmt.Printf("Lightning Address Adoption: %.0f%%\n", generator.LightningAdoptionRate*100)
	fmt.Printf("DHT Key Adoption: %.0f%%\n", generator.DHTKeyAdoptionRate*100)
	fmt.Printf("Both Adoption: %.0f%%\n", generator.BothAdoptionRate*100)

	fmt.Printf("Initial Notes Per Day: %d\n", generator.InitialNotesPerDay)
	fmt.Printf("Notes Growth Rate: %.0f%% (factor: %.2f)\n",
		(generator.NotesGrowthRate-1.0)*100, generator.NotesGrowthRate)
	fmt.Printf("Media Percentage: %.0f%%\n", generator.MediaPercentage*100)

	fmt.Println("\nKind Distribution:")
	for kind, percentage := range generator.KindDistribution {
		fmt.Printf("- Kind %d: %.0f%%\n", kind, percentage*100)
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
