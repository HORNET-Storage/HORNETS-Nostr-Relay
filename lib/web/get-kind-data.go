package web

import (
	"log"
	"sort"

	types "github.com/HORNET-Storage/hornet-storage/lib"
	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type AggregatedKindData struct {
	KindNumber int     `json:"kindNumber"`
	KindCount  int     `json:"kindCount"`
	TotalSize  float64 `json:"totalSize"`
}

func handleKindData(c *fiber.Ctx) error {
	log.Println("Kind data request received")

	db, err := gorm.Open(sqlite.Open("relay_stats.db"), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var kinds []types.Kind
	if err := db.Find(&kinds).Error; err != nil {
		log.Println("Error fetching kinds:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	aggregatedData := make(map[int]AggregatedKindData)

	for _, kind := range kinds {
		if data, exists := aggregatedData[kind.KindNumber]; exists {
			data.KindCount++
			data.TotalSize += kind.Size
			aggregatedData[kind.KindNumber] = data
		} else {
			aggregatedData[kind.KindNumber] = AggregatedKindData{
				KindNumber: kind.KindNumber,
				KindCount:  1,
				TotalSize:  kind.Size,
			}
		}
	}

	result := []AggregatedKindData{}
	for _, data := range aggregatedData {
		result = append(result, data)
	}

	// Sort by TotalSize in descending order
	sort.Slice(result, func(i, j int) bool {
		return result[i].TotalSize > result[j].TotalSize
	})

	return c.JSON(result)
}
