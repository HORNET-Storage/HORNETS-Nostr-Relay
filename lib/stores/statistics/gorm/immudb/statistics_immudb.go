package immudb

import (
	"context"
	"fmt"

	statistics_gorm "github.com/HORNET-Storage/hornet-storage/lib/stores/statistics/gorm"
	"github.com/codenotary/immudb/pkg/api/schema"
	immudb "github.com/codenotary/immudb/pkg/client"
	immugorm "github.com/codenotary/immugorm"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Specific init for immudb
func InitStore(args ...interface{}) (*statistics_gorm.GormStatisticsStore, error) {
	client := args[0].(immudb.ImmuClient)

	createDatabaseIfNotExists(client, "statistics")

	store := &statistics_gorm.GormStatisticsStore{}

	username := "immudb"
	password := "immudb"
	host := "127.0.0.1"
	port := "3322"
	database := "statistics"

	if len(args) >= 5 {
		if u, ok := args[0].(string); ok {
			username = u
		}
		if p, ok := args[1].(string); ok {
			password = p
		}
		if h, ok := args[2].(string); ok {
			host = h
		}
		if p, ok := args[3].(string); ok {
			port = p
		}
		if d, ok := args[4].(string); ok {
			database = d
		}
	}

	dsn := fmt.Sprintf("immudb://%s:%s@%s:%s/%s?sslmode=disable", username, password, host, port, database)

	var err error
	store.DB, err = gorm.Open(immugorm.Open(dsn, &immugorm.ImmuGormConfig{Verify: false}), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Info),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to connect to immudb: %v", err)
	}

	err = store.Init()
	if err != nil {
		return nil, err
	}

	return store, nil
}

func createDatabaseIfNotExists(client immudb.ImmuClient, dbName string) error {
	ctx := context.Background()
	// Use the system database to create new databases
	_, err := client.UseDatabase(ctx, &schema.Database{
		DatabaseName: "systemdb",
	})
	if err != nil {
		return fmt.Errorf("failed to use systemdb: %w", err)
	}

	_, err = client.CreateDatabaseV2(ctx, dbName, &schema.DatabaseNullableSettings{})
	if err != nil {
		return fmt.Errorf("failed to create database: %w", err)
	}

	return nil
}
