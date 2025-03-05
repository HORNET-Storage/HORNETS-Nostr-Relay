package sqlite

import (
	"fmt"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	statistics_gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm"
)

func InitStore(args ...interface{}) (*statistics_gorm.GormStatisticsStore, error) {
	store := &statistics_gorm.GormStatisticsStore{}

	var err error

	store.DB, err = gorm.Open(sqlite.Open("statistics.db"), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to immudb: %v", err)
	}

	err = store.Init()
	if err != nil {
		return nil, err
	}

	return store, nil
}
