package web

import (
	"log"
	"sort"
	"strconv"
	"time"

	"github.com/gofiber/fiber/v2"
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

func getKindTrendData(c *fiber.Ctx) error {
	log.Println("Kind trend data request received")
	kindNumberStr := c.Params("kindNumber")
	kindNumber, err := strconv.Atoi(kindNumberStr)
	if err != nil {
		log.Printf("Error converting kind number to integer: %v", err)
		return c.Status(fiber.StatusBadRequest).SendString("Invalid kind number")
	}

	// Retrieve the gorm db
	db := c.Locals("db").(*gorm.DB)

	var data []KindData
	query := `
		SELECT timestamp, size
		FROM kinds
		WHERE kind_number = ? AND timestamp >= date('now', '-12 months')
	`
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

	return c.JSON(result)
}
