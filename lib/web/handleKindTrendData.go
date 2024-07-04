package web

import (
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type KindData struct {
	Month     string
	Size      float64
	Timestamp time.Time
}

type MonthlyKindData struct {
	Month     string  `json:"month"`
	TotalSize float64 `json:"totalSize"`
}

func handleKindTrendData(c *fiber.Ctx) error {
	log.Println("Kind trend data request received")
	kindNumberStr := c.Params("kindNumber")
	kindNumber, err := strconv.Atoi(kindNumberStr)
	if err != nil {
		log.Printf("Error converting kind number to integer: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Invalid kind number")
	}
	log.Println("Kind number:", kindNumber)

	db, err := gorm.Open(sqlite.Open("relay_stats.db"), &gorm.Config{})
	if err != nil {
		log.Printf("Failed to connect to the database: %v", err)
		return c.Status(fiber.StatusInternalServerError).SendString("Internal Server Error")
	}

	var data []KindData
	query := `
		SELECT timestamp, size
		FROM kinds
		WHERE kind_number = ? AND timestamp >= date('now', '-12 months')
	`
	log.Printf("Executing query: %s with kindNumber: %d", query, kindNumber)
	err = db.Raw(query, kindNumber).Scan(&data).Error

	if err != nil {
		log.Println("Error fetching kind data:", err)
		return c.Status(500).SendString("Internal Server Error")
	}

	if len(data) == 0 {
		log.Println("No data found for the specified kind number and time range")
		return c.Status(404).SendString("No data found")
	}

	// Manually sum the sizes per month
	monthlyData := make(map[string]float64)
	for _, row := range data {
		month := row.Timestamp.Format("2006-01")
		monthlyData[month] += row.Size
	}

	var result []MonthlyKindData
	for month, totalSize := range monthlyData {
		result = append(result, MonthlyKindData{Month: month, TotalSize: totalSize})
	}

	// Sort the result by month
	sort.Slice(result, func(i, j int) bool {
		return result[i].Month < result[j].Month
	})

	log.Printf("Query returned %d rows", len(data))
	for _, row := range result {
		log.Printf("Month: %s, TotalSize: %f", row.Month, row.TotalSize)
	}

	return c.JSON(result)
}
