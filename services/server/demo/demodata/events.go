package demodata

import (
	"fmt"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
	jsoniter "github.com/json-iterator/go"
	"github.com/nbd-wtf/go-nostr"
)

// GenerateEventKinds creates event kind statistics data
func (g *DemoDataGenerator) GenerateEventKinds(store statistics.StatisticsStore) error {
	fmt.Println("Generating event kind statistics month by month...")

	// Calculate kinds per day for each month
	kindsPerDay := g.InitialNotesPerDay

	// We have these kinds in our distribution map: 1, 3, 4, 5, 7, 1063, 30023

	// Generate data for each month in the range
	currentMonth := g.StartMonth
	for !currentMonth.After(g.EndMonth) {
		fmt.Printf("Generating kind statistics for %s...\n", currentMonth.Format("Jan 2006"))

		// Calculate days in this month
		daysInMonthCount := daysInMonth(currentMonth)

		// Calculate total kinds for this month
		totalKinds := kindsPerDay * daysInMonthCount

		// Process in batches
		batchSize := 100
		for i := 0; i < totalKinds; i += batchSize {
			// Calculate actual batch size
			currentBatchSize := batchSize
			if i+currentBatchSize > totalKinds {
				currentBatchSize = totalKinds - i
			}

			// Generate kinds for this batch
			for j := 0; j < currentBatchSize; j++ {
				// Determine the kind based on distribution
				kindNumber := g.selectRandomKind()

				// Determine if this is a media kind
				isMedia := g.isMediaKind(kindNumber) || (kindNumber == 1 && g.rng.Float64() < g.MediaPercentage)

				// Generate size based on kind and media status
				sizeMB := g.generateSizeForKind(kindNumber, isMedia)

				// Create a timestamp within this month
				timestamp := getRandomTimeInMonth(currentMonth, g.rng)

				// Create Kind entry directly
				kind := &lib.Kind{
					KindNumber:       kindNumber,
					EventID:          g.generateEventID(),
					TimestampHornets: timestamp,
					Size:             sizeMB,
				}

				// Use the SaveKind method to store the Kind entry
				if err := g.saveKind(store, kind); err != nil {
					return fmt.Errorf("error saving kind: %v", err)
				}

				// If this is a media kind, generate file info entry
				if isMedia {
					if err := g.generateFileInfo(store, kind, timestamp); err != nil {
						return fmt.Errorf("error generating file info: %v", err)
					}
				}
			}
		}

		// Increase kinds per day for next month based on growth rate
		kindsPerDay = int(float64(kindsPerDay) * g.NotesGrowthRate)

		// Move to next month
		currentMonth = addMonth(currentMonth)
	}

	fmt.Println("Event kind statistics generation complete!")
	return nil
}

// saveKind saves a Kind directly to the statistics store
func (g *DemoDataGenerator) saveKind(store statistics.StatisticsStore, kind *lib.Kind) error {
	// Create a valid nostr.Event with the minimum required fields
	// Based on the implementation in statistics_gorm.go
	pubKey := g.generatePubKey()
	event := &nostr.Event{
		ID:        kind.EventID,
		Kind:      kind.KindNumber,
		CreatedAt: nostr.Timestamp(kind.TimestampHornets.Unix()),
		PubKey:    pubKey,
		Tags:      make(nostr.Tags, 0),   // Empty tags array
		Content:   "",                    // Empty content
		Sig:       generateRandomHex(64), // Generate a mock signature
	}

	// For Kind 0 (metadata), add proper content
	if kind.KindNumber == 0 {
		// Determine if this profile has lightning and/or DHT
		hasLightning := g.rng.Float64() < g.LightningAdoptionRate
		hasDHT := g.rng.Float64() < g.DHTKeyAdoptionRate

		// If both adoption is specified, override individual settings sometimes
		if g.rng.Float64() < g.BothAdoptionRate {
			hasLightning = true
			hasDHT = true
		}

		// Create profile content
		profile := map[string]interface{}{
			"name":    "User " + pubKey[:8],
			"about":   "Demo user profile",
			"picture": "https://robohash.org/" + pubKey + ".png",
		}

		// Add lightning address if needed
		if hasLightning {
			profile["nip05"] = pubKey[:8] + "@demo.com"
		}

		// Add DHT key if needed
		if hasDHT {
			profile["dht-key"] = "dht:" + generateRandomHex(32)
		}

		// Convert to JSON
		contentBytes, _ := jsoniter.Marshal(profile)
		event.Content = string(contentBytes)
	}

	// Call the SaveEventKind method
	if err := store.SaveEventKind(event); err != nil {
		return err
	}

	return nil
}

// generateFileInfo creates a FileInfo entry for media content
func (g *DemoDataGenerator) generateFileInfo(store statistics.StatisticsStore, kind *lib.Kind, _ time.Time) error {
	// Generate file metadata
	mimeType := g.generateMimeType(kind.KindNumber)
	fileName := g.generateFileName(mimeType)

	// Convert MB to bytes for file size
	sizeBytes := int64(kind.Size * 1024 * 1024)

	// Create a file entry
	if err := store.SaveFile(
		kind.EventID,     // Use event ID as root
		g.generateHash(), // Generate a hash
		fileName,         // Generated file name
		mimeType,         // Generated MIME type
		g.rng.Intn(10)+1, // Random leaf count between 1-10
		sizeBytes,        // Size in bytes
	); err != nil {
		return err
	}

	return nil
}
