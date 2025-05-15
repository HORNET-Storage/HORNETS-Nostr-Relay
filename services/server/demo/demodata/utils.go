package demodata

import (
	cryptorand "crypto/rand"
	"encoding/hex"
	"math/rand"
	"time"
)

// getRandomTimeInMonth generates a random timestamp within the given month
func getRandomTimeInMonth(month time.Time, rng *rand.Rand) time.Time {
	// Get the first day of the month
	firstDay := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())

	// Get the first day of the next month
	var nextMonth time.Month
	var nextYear int
	if month.Month() == time.December {
		nextMonth = time.January
		nextYear = month.Year() + 1
	} else {
		nextMonth = month.Month() + 1
		nextYear = month.Year()
	}
	nextMonthFirstDay := time.Date(nextYear, nextMonth, 1, 0, 0, 0, 0, month.Location())

	// Calculate the duration of the month in nanoseconds
	monthDuration := nextMonthFirstDay.Sub(firstDay)

	// Generate a random duration within the month
	randomDuration := time.Duration(rng.Int63n(int64(monthDuration)))

	// Return the random time
	return firstDay.Add(randomDuration)
}

// addMonth adds one month to the given time
func addMonth(t time.Time) time.Time {
	// Get the current year and month
	year, month, day := t.Date()

	// Calculate the next month
	nextMonth := month + 1
	nextYear := year
	if nextMonth > 12 {
		nextMonth = 1
		nextYear++
	}

	// Create a new time with the next month
	// Handle cases where the day might not exist in the next month
	// (e.g., January 31 -> February 28/29)
	nextMonthFirstDay := time.Date(nextYear, nextMonth, 1, 0, 0, 0, 0, t.Location())
	nextMonthLastDay := nextMonthFirstDay.AddDate(0, 1, -1).Day()
	if day > nextMonthLastDay {
		day = nextMonthLastDay
	}

	return time.Date(nextYear, nextMonth, day, 0, 0, 0, 0, t.Location())
}

// addMonths adds the specified number of months to the given time
func addMonths(t time.Time, months int) time.Time {
	result := t
	for i := 0; i < months; i++ {
		result = addMonth(result)
	}
	return result
}

// monthsBetween calculates the number of months between two dates
func monthsBetween(start, end time.Time) int {
	months := 0
	current := start

	for !current.After(end) {
		months++
		current = addMonth(current)
	}

	return months - 1 // Subtract 1 because we want the range, not the count
}

// daysInMonth calculates the number of days in a month
func daysInMonth(month time.Time) int {
	// Get the first day of the month
	firstDay := time.Date(month.Year(), month.Month(), 1, 0, 0, 0, 0, month.Location())

	// Get the first day of the next month
	var nextMonth time.Month
	var nextYear int
	if month.Month() == time.December {
		nextMonth = time.January
		nextYear = month.Year() + 1
	} else {
		nextMonth = month.Month() + 1
		nextYear = month.Year()
	}
	nextMonthFirstDay := time.Date(nextYear, nextMonth, 1, 0, 0, 0, 0, month.Location())

	// Calculate the difference in days
	return int(nextMonthFirstDay.Sub(firstDay).Hours() / 24)
}

// generateRandomHex generates a random hex string of the specified length
func generateRandomHex(length int) string {
	bytes := make([]byte, length/2)
	_, _ = cryptorand.Read(bytes)
	return hex.EncodeToString(bytes)
}
