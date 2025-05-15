package demodata

import (
	"fmt"
	"math"
	"math/rand"
	"strings"
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
		MediaPercentage:       0.5,  // 50% of notes are media - increased to get more media content

		// Kind distribution - percentages should sum to 1.0
		// Only using kinds that are in the whitelist
		KindDistribution: map[int]float64{
			0:     0.05, // Metadata/Profiles: 5%
			1:     0.30, // Text notes: 30%
			2:     0.05, // Recommended relay: 5%
			3:     0.05, // Contacts: 5%
			5:     0.05, // Deletions: 5%
			6:     0.05, // Reposts: 5%
			7:     0.10, // Reactions: 10%
			8:     0.05, // Badges: 5%
			16:    0.05, // Generic events: 5%
			30023: 0.10, // Long-form content: 10% (increased for better media representation)
			10000: 0.05, // Mute lists: 5%
			10001: 0.05, // Pin lists: 5%
			30000: 0.05, // Categorized people lists: 5%
		},

		// Size ranges for different kinds (in MB)
		KindSizes: map[int]struct {
			Min float64
			Max float64
		}{
			0:     {0.0005, 0.002},  // Metadata: 500B to 2KB
			1:     {0.0005, 0.005},  // Text notes: 500B to 5KB
			2:     {0.0003, 0.001},  // Recommended relay: 300B to 1KB
			3:     {0.001, 0.01},    // Contacts: 1KB to 10KB
			5:     {0.0001, 0.0005}, // Deletions: 100B to 500B
			6:     {0.0005, 0.003},  // Reposts: 500B to 3KB
			7:     {0.0001, 0.0005}, // Reactions: 100B to 500B
			8:     {0.0005, 0.002},  // Badges: 500B to 2KB
			16:    {0.001, 0.01},    // Generic events: 1KB to 10KB
			30023: {0.05, 3.0},      // Long-form content: 50KB to 3MB (increased size for media content)
			10000: {0.001, 0.005},   // Mute lists: 1KB to 5KB
			10001: {0.001, 0.005},   // Pin lists: 1KB to 5KB
			30000: {0.001, 0.01},    // Categorized people lists: 1KB to 10KB
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

	// Generate wallet balance history
	if err := g.GenerateWalletBalance(store); err != nil {
		return fmt.Errorf("error generating wallet balance: %v", err)
	}

	// Generate wallet transactions (100 transactions)
	if err := g.GenerateWalletTransactions(store, 100); err != nil {
		return fmt.Errorf("error generating wallet transactions: %v", err)
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
	// These kinds typically contain media (focused on kinds in our whitelist)
	mediaKinds := map[int]bool{
		1:     false, // Text notes (handled separately with MediaPercentage)
		6:     true,  // Reposts often include media
		8:     true,  // Badges often include images
		16:    true,  // Generic events can contain rich media
		30023: true,  // Long-form content (articles)
		10000: false, // Mute lists
		10001: true,  // Pin lists often include media items
		30000: true,  // Categorized people lists can include rich content
	}

	// For kinds explicitly marked as media
	if result, exists := mediaKinds[kind]; exists {
		return result
	}

	// For other kinds, give a 30% chance for them to contain media
	// This ensures even less common kinds have a chance to include media
	return g.rng.Float64() < 0.3
}

// generateSizeForKind generates a realistic size for a given kind
func (g *DemoDataGenerator) generateSizeForKind(kind int, isMedia bool) float64 {
	// Get size range for this kind
	sizeRange, exists := g.KindSizes[kind]
	if !exists {
		// Default ranges if not specified
		if isMedia {
			// Check what type of media and assign appropriate size range
			mimeType := g.generateMimeType(kind)

			if strings.HasPrefix(mimeType, "video/") {
				// Videos: 5MB to 50MB
				sizeRange = struct {
					Min float64
					Max float64
				}{5.0, 50.0}
			} else if strings.HasPrefix(mimeType, "audio/") {
				// Audio: 1MB to 15MB
				sizeRange = struct {
					Min float64
					Max float64
				}{1.0, 15.0}
			} else if strings.HasPrefix(mimeType, "image/") {
				// Images: 0.2MB to 5MB
				sizeRange = struct {
					Min float64
					Max float64
				}{0.2, 5.0}
			} else {
				// Default for other media: 0.2MB to 2MB
				sizeRange = struct {
					Min float64
					Max float64
				}{0.2, 2.0}
			}
		} else {
			// Non-media content
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
func (g *DemoDataGenerator) generateMimeType(_ int) string {
	// Default distribution for any kind that has media
	// 40% images, 30% videos, 25% audio, 5% documents
	// Balanced to provide more video and audio content
	r := g.rng.Float64()

	// Different categories
	if r < 0.40 {
		// Images (40%)
		imageTypes := []string{"image/jpeg", "image/png", "image/gif", "image/webp"}
		return imageTypes[g.rng.Intn(len(imageTypes))]
	} else if r < 0.70 {
		// Videos (30%)
		videoTypes := []string{"video/mp4", "video/webm", "video/quicktime"}
		return videoTypes[g.rng.Intn(len(videoTypes))]
	} else if r < 0.95 {
		// Audio (25%)
		audioTypes := []string{"audio/mpeg", "audio/wav", "audio/ogg"}
		return audioTypes[g.rng.Intn(len(audioTypes))]
	} else {
		// Documents (5%)
		docTypes := []string{"application/pdf", "application/zip", "text/plain", "application/json"}
		return docTypes[g.rng.Intn(len(docTypes))]
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
