package demodata

import (
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores/statistics"
)

// DemoDataGenerator generates realistic data for the demo mode
type DemoDataGenerator struct {
	// Configuration options
	RandomSeed int64
	rng        *rand.Rand

	// Time range settings
	StartMonth time.Time // Start of data generation period
	EndMonth   time.Time // End of data generation period (defaults to current month)

	// Growth patterns
	InitialUserCount      int     // Starting number of users
	MonthlyUserGrowthRate float64 // Monthly user growth rate (e.g., 1.2 = 20% growth)
	LightningAdoptionRate float64 // Rate of lightning address adoption
	DHTKeyAdoptionRate    float64 // Rate of DHT key adoption
	BothAdoptionRate      float64 // Rate of users with both lightning and DHT

	// Note patterns
	InitialNotesPerDay int     // Starting number of notes per day
	NotesGrowthRate    float64 // Growth rate of notes per month
	MediaPercentage    float64 // Percentage of notes that are media

	// Kind distributions
	KindDistribution map[int]float64 // Distribution of different kinds (kind number -> percentage)

	// Size distributions (in MB)
	KindSizes map[int]struct {
		Min float64
		Max float64
	}

	// Cached generated data
	pubkeys  []string
	eventIDs []string
}

// NewDemoDataGenerator creates a new generator with default settings
func NewDemoDataGenerator() *DemoDataGenerator {
	// Default to 24 months of data ending at current month
	endMonth := time.Now()
	startMonth := endMonth.AddDate(-2, 0, 0) // 2 years ago

	seed := time.Now().UnixNano()

	gen := &DemoDataGenerator{
		RandomSeed:            seed,
		rng:                   rand.New(rand.NewSource(seed)),
		StartMonth:            startMonth,
		EndMonth:              endMonth,
		InitialUserCount:      100,
		MonthlyUserGrowthRate: 1.15, // 15% monthly growth
		LightningAdoptionRate: 0.3,  // 30% of users have lightning
		DHTKeyAdoptionRate:    0.2,  // 20% of users have DHT keys
		BothAdoptionRate:      0.1,  // 10% have both
		InitialNotesPerDay:    50,   // 50 notes per day initially
		NotesGrowthRate:       1.1,  // 10% monthly growth in notes
		MediaPercentage:       0.2,  // 20% of notes are media

		// Kind distribution - percentages should sum to 1.0
		KindDistribution: map[int]float64{
			0:     0.10, // Metadata/Profiles: 10%
			1:     0.45, // Text notes: 45%
			3:     0.10, // Contacts: 10%
			4:     0.05, // Direct messages: 5%
			5:     0.05, // Deletions: 5%
			7:     0.15, // Reactions: 15%
			1063:  0.05, // File metadata: 5%
			30023: 0.05, // Article: 5%
		},

		// Size ranges for different kinds (in MB)
		KindSizes: map[int]struct {
			Min float64
			Max float64
		}{
			0:     {0.0005, 0.002},  // Metadata: 500B to 2KB
			1:     {0.0005, 0.005},  // Text notes: 500B to 5KB
			3:     {0.001, 0.01},    // Contacts: 1KB to 10KB
			4:     {0.0005, 0.002},  // DMs: 500B to 2KB
			5:     {0.0001, 0.0005}, // Deletions: 100B to 500B
			7:     {0.0001, 0.0005}, // Reactions: 100B to 500B
			1063:  {0.5, 5.0},       // File metadata: 500KB to 5MB
			30023: {0.05, 0.5},      // Article: 50KB to 500KB
		},

		pubkeys:  make([]string, 0, 1000),
		eventIDs: make([]string, 0, 5000),
	}

	return gen
}

// SetSeed sets a specific random seed for reproducible generation
func (g *DemoDataGenerator) SetSeed(seed int64) {
	g.RandomSeed = seed
	g.rng = rand.New(rand.NewSource(seed))
}

// GenerateAllData generates all types of demo data
func (g *DemoDataGenerator) GenerateAllData(store statistics.StatisticsStore) error {
	// Generate user profiles
	if err := g.GenerateUserProfiles(store); err != nil {
		return fmt.Errorf("error generating user profiles: %v", err)
	}

	// Generate a reduced amount of event kinds for better performance
	// and to avoid duplicate logs filling the console
	g.InitialNotesPerDay = 5 // Reduced from the default (usually 50)

	// Generate event kinds and files
	if err := g.GenerateEventKinds(store); err != nil {
		return fmt.Errorf("error generating event kinds: %v", err)
	}

	// Generate a limited number of payment notifications
	paymentCount := 20 // Fixed small number instead of percentage of users
	if err := g.GeneratePaymentNotifications(store, paymentCount); err != nil {
		return fmt.Errorf("error generating payment notifications: %v", err)
	}

	return nil
}

// Helper methods for random data generation
func (g *DemoDataGenerator) generatePubKey() string {
	// 80% chance to reuse an existing pubkey if we have some
	if len(g.pubkeys) > 0 && g.rng.Float64() < 0.8 {
		return g.pubkeys[g.rng.Intn(len(g.pubkeys))]
	}

	// Generate a new pubkey (hex string of length 64)
	pubkey := generateRandomHex(64)

	// Add to cache
	g.pubkeys = append(g.pubkeys, pubkey)

	return pubkey
}

func (g *DemoDataGenerator) generateEventID() string {
	// 5% chance to reuse an existing event ID if we have some
	// (Reduced from 70% to minimize duplicate event warnings)
	if len(g.eventIDs) > 0 && g.rng.Float64() < 0.05 {
		return g.eventIDs[g.rng.Intn(len(g.eventIDs))]
	}

	// Generate a new event ID (hex string of length 64)
	eventID := generateRandomHex(64)

	// Add to cache
	g.eventIDs = append(g.eventIDs, eventID)

	return eventID
}

func (g *DemoDataGenerator) generateHash() string {
	return generateRandomHex(64)
}

func (g *DemoDataGenerator) generateTxID() string {
	return generateRandomHex(64)
}

func (g *DemoDataGenerator) selectRandomKind() int {
	// Use the kind distribution to select a random kind
	r := g.rng.Float64()
	cumulative := 0.0

	for kind, prob := range g.KindDistribution {
		cumulative += prob
		if r < cumulative {
			return kind
		}
	}

	// Default to kind 1 if something goes wrong
	return 1
}

// isMediaKind determines if a kind is typically associated with media
func (g *DemoDataGenerator) isMediaKind(kind int) bool {
	// These kinds typically contain media
	mediaKinds := map[int]bool{
		1063:  true, // File metadata
		30023: true, // Article with images
		30063: true, // Live chat message with media
		1000:  true, // Regular post with media
	}
	return mediaKinds[kind]
}

// generateSizeForKind generates a realistic size for a given kind
func (g *DemoDataGenerator) generateSizeForKind(kind int, isMedia bool) float64 {
	// Get size range for this kind
	sizeRange, exists := g.KindSizes[kind]
	if !exists {
		// Default ranges if not specified
		if isMedia {
			sizeRange = struct {
				Min float64
				Max float64
			}{0.2, 2.0} // 200KB to 2MB for media
		} else {
			sizeRange = struct {
				Min float64
				Max float64
			}{0.0005, 0.01} // 500B to 10KB for text
		}
	}

	// Generate a random size within the range
	// Use exponential distribution to favor smaller sizes
	randomFactor := math.Pow(g.rng.Float64(), 2) // Square to bias toward smaller values
	return sizeRange.Min + randomFactor*(sizeRange.Max-sizeRange.Min)
}

// generateMimeType generates a realistic MIME type for a file
func (g *DemoDataGenerator) generateMimeType(kind int) string {
	// Different MIME types based on kind
	if kind == 1063 {
		// File metadata - various file types
		fileTypes := []string{
			"image/jpeg", "image/png", "image/gif",
			"video/mp4", "audio/mpeg", "application/pdf",
			"application/zip", "text/plain", "application/json",
		}
		return fileTypes[g.rng.Intn(len(fileTypes))]
	} else if kind == 30023 {
		// Article - mostly images
		imageTypes := []string{"image/jpeg", "image/png", "image/webp"}
		return imageTypes[g.rng.Intn(len(imageTypes))]
	} else {
		// Default to common image types for other kinds
		return "image/jpeg"
	}
}

// generateFileName generates a realistic file name based on MIME type
func (g *DemoDataGenerator) generateFileName(mimeType string) string {
	// Generate a random name
	prefix := "file_"
	timestamp := time.Now().Unix()
	random := g.rng.Intn(10000)

	// Add appropriate extension based on MIME type
	var extension string
	switch mimeType {
	case "image/jpeg":
		extension = ".jpg"
	case "image/png":
		extension = ".png"
	case "image/gif":
		extension = ".gif"
	case "image/webp":
		extension = ".webp"
	case "video/mp4":
		extension = ".mp4"
	case "audio/mpeg":
		extension = ".mp3"
	case "application/pdf":
		extension = ".pdf"
	case "application/zip":
		extension = ".zip"
	case "text/plain":
		extension = ".txt"
	case "application/json":
		extension = ".json"
	default:
		extension = ".bin"
	}

	return fmt.Sprintf("%s%d_%d%s", prefix, timestamp, random, extension)
}
