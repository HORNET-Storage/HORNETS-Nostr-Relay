package contentmoderation

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"gorm.io/gorm"
)

// GormStore implements the Store interface using GORM
type GormStore struct {
	DB            *gorm.DB
	config        *Config
	tempDir       string
	cacheLock     sync.RWMutex
	cache         map[string]*ModerationRecord
	eventLock     sync.RWMutex
	eventCache    map[string]EventStatus
	externalLock  sync.RWMutex
	externalCache map[string]ContentStatus
}

// GormModerationRecord is the GORM model for content moderation records
type GormModerationRecord struct {
	gorm.Model
	DagRoot         string    `gorm:"unique;not null"`
	ContentType     string    `gorm:"not null"`
	FileSize        int64     `gorm:"not null"`
	UploadedBy      string    `gorm:"not null"`
	UploadedAt      time.Time `gorm:"not null"`
	Status          string    `gorm:"not null"`
	ModeratedAt     *time.Time
	ModerationLevel int
	ModerationData  string // JSON encoded
	AttemptCount    int    `gorm:"not null"`
	ProcessingError string
	NextAttemptAt   *time.Time
}

// GormEventMediaReference is the GORM model for event-to-media references
type GormEventMediaReference struct {
	gorm.Model
	EventID     string    `gorm:"not null"`
	MediaURL    string    `gorm:"not null"`
	MediaID     string    `gorm:"not null"`
	SourceType  string    `gorm:"not null"`
	ReferenceAt time.Time `gorm:"not null"`
}

// GormEventModerationStatus is the GORM model for event moderation status
type GormEventModerationStatus struct {
	gorm.Model
	EventID   string    `gorm:"unique;not null"`
	Status    string    `gorm:"not null"`
	CheckedAt time.Time `gorm:"not null"`
}

// GormExternalMediaCache is the GORM model for external media cache
type GormExternalMediaCache struct {
	gorm.Model
	URL         string    `gorm:"unique;not null"`
	Status      string    `gorm:"not null"`
	CheckedAt   time.Time `gorm:"not null"`
	FileHash    string    `gorm:"not null"`
	ContentType string    `gorm:"not null"`
}

// NewGormStore creates a new GormStore
func NewGormStore(db *gorm.DB, config *Config) (*GormStore, error) {
	// Ensure temp directory exists
	if err := os.MkdirAll(config.TempStoragePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	store := &GormStore{
		DB:            db,
		config:        config,
		tempDir:       config.TempStoragePath,
		cache:         make(map[string]*ModerationRecord),
		eventCache:    make(map[string]EventStatus),
		externalCache: make(map[string]ContentStatus),
	}

	return store, nil
}

// Initialize sets up the database tables
func (s *GormStore) Initialize() error {
	// Auto migrate the tables
	if err := s.DB.AutoMigrate(
		&GormModerationRecord{},
		&GormEventMediaReference{},
		&GormEventModerationStatus{},
		&GormExternalMediaCache{},
	); err != nil {
		return fmt.Errorf("failed to migrate tables: %w", err)
	}

	// Create indexes for better performance
	// These operations are idempotent - safe to run multiple times
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_content_moderation_records_status ON content_moderation_records(status)")
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_content_moderation_records_dag_root ON content_moderation_records(dag_root)")
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_event_media_references_event_id ON event_media_references(event_id)")
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_event_media_references_media_id ON event_media_references(media_id)")
	s.DB.Exec("CREATE INDEX IF NOT EXISTS idx_external_media_cache_checked_at ON external_media_cache(checked_at)")

	return nil
}

// Close cleans up resources
func (s *GormStore) Close() error {
	// Nothing to clean up with GORM
	return nil
}

// EnqueueMedia adds a media item to the moderation queue
func (s *GormStore) EnqueueMedia(record *ModerationRecord, data []byte) error {
	if record == nil {
		return errors.New("nil record")
	}
	if len(data) == 0 {
		return errors.New("empty media data")
	}

	// Set default values
	now := time.Now()
	record.Status = StatusAwaiting
	record.CreatedAt = now
	record.UpdatedAt = now
	record.AttemptCount = 0

	// Convert to GORM model
	gormRecord := GormModerationRecord{
		DagRoot:      record.DagRoot,
		ContentType:  record.ContentType,
		FileSize:     record.FileSize,
		UploadedBy:   record.UploadedBy,
		UploadedAt:   record.UploadedAt,
		Status:       string(record.Status),
		AttemptCount: record.AttemptCount,
	}

	// Save to database
	if err := s.DB.Create(&gormRecord).Error; err != nil {
		return fmt.Errorf("failed to create record: %w", err)
	}

	// Add to cache
	s.cacheLock.Lock()
	s.cache[record.DagRoot] = record
	s.cacheLock.Unlock()

	// Store the file in temporary storage
	filePath := GenerateTempFilePath(record.DagRoot, record.ContentType, s.config)
	if err := os.WriteFile(filePath, data, 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	if s.config.Debug {
		log.Printf("Enqueued media %s (%s) for moderation", record.DagRoot, record.ContentType)
	}

	return nil
}

// GetMediaRecord retrieves a moderation record
func (s *GormStore) GetMediaRecord(dagRoot string) (*ModerationRecord, error) {
	// Check cache first
	s.cacheLock.RLock()
	if record, ok := s.cache[dagRoot]; ok {
		s.cacheLock.RUnlock()
		return record, nil
	}
	s.cacheLock.RUnlock()

	// Not in cache, query database
	var gormRecord GormModerationRecord
	if err := s.DB.Where("dag_root = ?", dagRoot).First(&gormRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("failed to get record: %w", err)
	}

	// Convert to ModerationRecord
	record := &ModerationRecord{
		DagRoot:         gormRecord.DagRoot,
		ContentType:     gormRecord.ContentType,
		FileSize:        gormRecord.FileSize,
		UploadedBy:      gormRecord.UploadedBy,
		UploadedAt:      gormRecord.UploadedAt,
		Status:          ContentStatus(gormRecord.Status),
		ModeratedAt:     gormRecord.ModeratedAt,
		ModerationLevel: gormRecord.ModerationLevel,
		AttemptCount:    gormRecord.AttemptCount,
		ProcessingError: gormRecord.ProcessingError,
		CreatedAt:       gormRecord.CreatedAt,
		UpdatedAt:       gormRecord.UpdatedAt,
	}

	// Add to cache
	s.cacheLock.Lock()
	s.cache[dagRoot] = record
	s.cacheLock.Unlock()

	return record, nil
}

// GetNextAwaitingMedia gets the next media to process
func (s *GormStore) GetNextAwaitingMedia() (*ModerationRecord, []byte, error) {
	var gormRecord GormModerationRecord

	// Start a transaction for safe status update
	tx := s.DB.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Find the next awaiting record
	if err := tx.Where("status = ?", string(StatusAwaiting)).
		Order("created_at ASC").
		First(&gormRecord).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNoMediaWaiting
		}
		return nil, nil, fmt.Errorf("failed to get awaiting record: %w", err)
	}

	// Update status to processing
	gormRecord.Status = string(StatusProcessing)
	gormRecord.AttemptCount++
	gormRecord.UpdatedAt = time.Now()
	if err := tx.Save(&gormRecord).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to update record status: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Convert to ModerationRecord
	record := &ModerationRecord{
		DagRoot:         gormRecord.DagRoot,
		ContentType:     gormRecord.ContentType,
		FileSize:        gormRecord.FileSize,
		UploadedBy:      gormRecord.UploadedBy,
		UploadedAt:      gormRecord.UploadedAt,
		Status:          ContentStatus(gormRecord.Status),
		ModeratedAt:     gormRecord.ModeratedAt,
		ModerationLevel: gormRecord.ModerationLevel,
		AttemptCount:    gormRecord.AttemptCount,
		ProcessingError: gormRecord.ProcessingError,
		CreatedAt:       gormRecord.CreatedAt,
		UpdatedAt:       gormRecord.UpdatedAt,
	}

	// Update cache
	s.cacheLock.Lock()
	s.cache[record.DagRoot] = record
	s.cacheLock.Unlock()

	// Get the file data
	filePath := GenerateTempFilePath(record.DagRoot, record.ContentType, s.config)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return record, nil, fmt.Errorf("failed to read file data: %w", err)
	}

	if s.config.Debug {
		log.Printf("Retrieved next awaiting media: %s", record.DagRoot)
	}

	return record, data, nil
}

// UpdateMediaRecord updates a moderation record
func (s *GormStore) UpdateMediaRecord(record *ModerationRecord) error {
	if record == nil {
		return errors.New("nil record")
	}

	// Update timestamp
	record.UpdatedAt = time.Now()

	// Convert to GORM model
	gormRecord := GormModerationRecord{
		DagRoot:         record.DagRoot,
		ContentType:     record.ContentType,
		FileSize:        record.FileSize,
		UploadedBy:      record.UploadedBy,
		UploadedAt:      record.UploadedAt,
		Status:          string(record.Status),
		ModeratedAt:     record.ModeratedAt,
		ModerationLevel: record.ModerationLevel,
		AttemptCount:    record.AttemptCount,
		ProcessingError: record.ProcessingError,
	}

	// Update in database
	result := s.DB.Model(&GormModerationRecord{}).
		Where("dag_root = ?", record.DagRoot).
		Updates(map[string]interface{}{
			"status":           gormRecord.Status,
			"moderated_at":     gormRecord.ModeratedAt,
			"moderation_level": gormRecord.ModerationLevel,
			"attempt_count":    gormRecord.AttemptCount,
			"processing_error": gormRecord.ProcessingError,
			"updated_at":       record.UpdatedAt,
		})

	if result.Error != nil {
		return fmt.Errorf("failed to update record: %w", result.Error)
	}

	if result.RowsAffected == 0 {
		// Record not found, insert it
		if err := s.DB.Create(&gormRecord).Error; err != nil {
			return fmt.Errorf("failed to create record: %w", err)
		}
	}

	// Update cache
	s.cacheLock.Lock()
	s.cache[record.DagRoot] = record
	s.cacheLock.Unlock()

	// If rejected, flag for deletion (but don't delete yet)
	if record.Status == StatusRejected {
		if s.config.Debug {
			log.Printf("Media %s marked for deletion", record.DagRoot)
		}
	}

	return nil
}

// DeleteRejectedMedia deletes rejected media
func (s *GormStore) DeleteRejectedMedia(dagRoot string) error {
	// Get the record
	record, err := s.GetMediaRecord(dagRoot)
	if err != nil {
		return err
	}

	// Verify it's rejected
	if record.Status != StatusRejected {
		return fmt.Errorf("cannot delete non-rejected media: %s", dagRoot)
	}

	// Delete the file
	filePath := GenerateTempFilePath(dagRoot, record.ContentType, s.config)
	if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete file: %w", err)
	}

	// Update status to deleted
	record.Status = StatusDeleted
	if err := s.UpdateMediaRecord(record); err != nil {
		return err
	}

	if s.config.Debug {
		log.Printf("Deleted rejected media: %s", dagRoot)
	}

	return nil
}

// SaveEventMediaReference saves an event-media reference
func (s *GormStore) SaveEventMediaReference(reference *EventMediaReference) error {
	if reference == nil {
		return errors.New("nil reference")
	}

	// Convert to GORM model
	gormReference := GormEventMediaReference{
		EventID:     reference.EventID,
		MediaURL:    reference.MediaURL,
		MediaID:     reference.MediaID,
		SourceType:  reference.SourceType,
		ReferenceAt: reference.ReferenceAt,
	}

	// Save to database (using upsert to avoid duplicates)
	result := s.DB.Where("event_id = ? AND media_url = ?", reference.EventID, reference.MediaURL).
		FirstOrCreate(&gormReference)

	if result.Error != nil {
		return fmt.Errorf("failed to save event media reference: %w", result.Error)
	}

	return nil
}

// GetEventMediaReferences gets media references for an event
func (s *GormStore) GetEventMediaReferences(eventID string) ([]MediaReferenceInfo, error) {
	var gormReferences []GormEventMediaReference
	var result []MediaReferenceInfo

	// Get references from database
	if err := s.DB.Where("event_id = ?", eventID).Find(&gormReferences).Error; err != nil {
		return nil, fmt.Errorf("failed to get event media references: %w", err)
	}

	// Convert to MediaReferenceInfo
	for _, ref := range gormReferences {
		info := MediaReferenceInfo{
			MediaURL:   ref.MediaURL,
			MediaID:    ref.MediaID,
			SourceType: ref.SourceType,
			Type:       "event_reference",
			Metadata:   make(map[string]string),
		}

		// If this is a scionic reference, add moderation status
		if ref.SourceType == "scionic" {
			if record, err := s.GetMediaRecord(ref.MediaID); err == nil {
				info.IsModerated = true
				info.Status = record.Status
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// MarkEventStatus sets the moderation status for an event
func (s *GormStore) MarkEventStatus(eventID string, status EventStatus) error {
	// Update cache
	s.eventLock.Lock()
	s.eventCache[eventID] = status
	s.eventLock.Unlock()

	// Create or update status
	now := time.Now()
	gormStatus := GormEventModerationStatus{
		EventID:   eventID,
		Status:    string(status),
		CheckedAt: now,
	}

	// Upsert in database
	result := s.DB.Where("event_id = ?", eventID).
		FirstOrCreate(&gormStatus)

	if result.Error != nil {
		return fmt.Errorf("failed to mark event status: %w", result.Error)
	}

	// If record exists, update it
	if result.RowsAffected == 0 {
		result = s.DB.Model(&GormEventModerationStatus{}).
			Where("event_id = ?", eventID).
			Updates(map[string]interface{}{
				"status":     string(status),
				"checked_at": now,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update event status: %w", result.Error)
		}
	}

	return nil
}

// GetEventStatus gets the moderation status for an event
func (s *GormStore) GetEventStatus(eventID string) (EventStatus, error) {
	// Check cache first
	s.eventLock.RLock()
	if status, ok := s.eventCache[eventID]; ok {
		s.eventLock.RUnlock()
		return status, nil
	}
	s.eventLock.RUnlock()

	// Not in cache, query database
	var gormStatus GormEventModerationStatus
	if err := s.DB.Where("event_id = ?", eventID).First(&gormStatus).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("event status not found")
		}
		return "", fmt.Errorf("failed to get event status: %w", err)
	}

	// Add to cache
	status := EventStatus(gormStatus.Status)
	s.eventLock.Lock()
	s.eventCache[eventID] = status
	s.eventLock.Unlock()

	return status, nil
}

// SaveExternalMediaCache saves a cache entry for external media
func (s *GormStore) SaveExternalMediaCache(url string, status ContentStatus, contentType string, fileHash string) error {
	// Update cache
	s.externalLock.Lock()
	s.externalCache[url] = status
	s.externalLock.Unlock()

	// Create cache entry
	now := time.Now()
	gormCache := GormExternalMediaCache{
		URL:         url,
		Status:      string(status),
		CheckedAt:   now,
		FileHash:    fileHash,
		ContentType: contentType,
	}

	// Upsert in database
	result := s.DB.Where("url = ?", url).
		FirstOrCreate(&gormCache)

	if result.Error != nil {
		return fmt.Errorf("failed to save external media cache: %w", result.Error)
	}

	// If record exists, update it
	if result.RowsAffected == 0 {
		result = s.DB.Model(&GormExternalMediaCache{}).
			Where("url = ?", url).
			Updates(map[string]interface{}{
				"status":       string(status),
				"checked_at":   now,
				"file_hash":    fileHash,
				"content_type": contentType,
			})

		if result.Error != nil {
			return fmt.Errorf("failed to update external media cache: %w", result.Error)
		}
	}

	return nil
}

// GetExternalMediaCache gets a cache entry for external media
func (s *GormStore) GetExternalMediaCache(url string) (*ExternalMediaCache, error) {
	// Check cache first
	s.externalLock.RLock()
	if status, ok := s.externalCache[url]; ok {
		s.externalLock.RUnlock()

		// Return a minimal object with just the status
		return &ExternalMediaCache{
			URL:       url,
			Status:    ContentStatus(string(status)),
			CheckedAt: time.Now(),
		}, nil
	}
	s.externalLock.RUnlock()

	// Not in cache, query database
	var gormCache GormExternalMediaCache
	if err := s.DB.Where("url = ?", url).First(&gormCache).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("external media cache not found")
		}
		return nil, fmt.Errorf("failed to get external media cache: %w", err)
	}

	// Check if expired
	if time.Since(gormCache.CheckedAt) > s.config.CacheTTL {
		return nil, fmt.Errorf("external media cache expired")
	}

	// Add to cache
	s.externalLock.Lock()
	s.externalCache[url] = ContentStatus(gormCache.Status)
	s.externalLock.Unlock()

	return &ExternalMediaCache{
		URL:         gormCache.URL,
		Status:      ContentStatus(gormCache.Status),
		CheckedAt:   gormCache.CheckedAt,
		FileHash:    gormCache.FileHash,
		ContentType: gormCache.ContentType,
	}, nil
}

// CleanupExpiredRecords removes old records and files
func (s *GormStore) CleanupExpiredRecords(cutoff time.Time) error {
	// Find rejected records older than cutoff
	var gormRecords []GormModerationRecord
	if err := s.DB.Where("status = ? AND updated_at < ?", string(StatusRejected), cutoff).
		Find(&gormRecords).Error; err != nil {
		return fmt.Errorf("failed to find expired records: %w", err)
	}

	// Delete each record's file
	for _, gormRecord := range gormRecords {
		// Create a record with minimal info just for path generation
		record := &ModerationRecord{
			DagRoot:     gormRecord.DagRoot,
			ContentType: gormRecord.ContentType,
		}

		filePath := GenerateTempFilePath(record.DagRoot, record.ContentType, s.config)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error deleting expired file %s: %v", filePath, err)
		}

		// Update status to deleted in the database directly
		if err := s.DB.Model(&gormRecord).Update("status", string(StatusDeleted)).Error; err != nil {
			log.Printf("Error updating record %s status: %v", gormRecord.DagRoot, err)
		}

		// Remove from cache
		s.cacheLock.Lock()
		delete(s.cache, gormRecord.DagRoot)
		s.cacheLock.Unlock()
	}

	// Find rejected files not linked to records (orphaned)
	dirEntries, err := os.ReadDir(s.tempDir)
	if err != nil {
		return fmt.Errorf("failed to read temp directory: %w", err)
	}

	for _, entry := range dirEntries {
		// Skip directories
		if entry.IsDir() {
			continue
		}

		// Get file info
		filePath := filepath.Join(s.tempDir, entry.Name())
		fileInfo, err := os.Stat(filePath)
		if err != nil {
			continue
		}

		// Check if file is older than cutoff
		if fileInfo.ModTime().Before(cutoff) {
			// Extract DAG root from filename
			dagRoot := filepath.Base(entry.Name())
			dagRoot = dagRoot[:len(dagRoot)-len(filepath.Ext(dagRoot))]

			// Check if there's a record for this file
			var count int64
			s.DB.Model(&GormModerationRecord{}).Where("dag_root = ?", dagRoot).Count(&count)
			if count == 0 {
				// Orphaned file, delete it
				if err := os.Remove(filePath); err != nil {
					log.Printf("Error deleting orphaned file %s: %v", filePath, err)
				} else if s.config.Debug {
					log.Printf("Deleted orphaned file: %s", filePath)
				}
			}
		}
	}

	// Expire external media cache
	if err := s.DB.Where("checked_at < ?", cutoff).Delete(&GormExternalMediaCache{}).Error; err != nil {
		log.Printf("Error cleaning up external media cache: %v", err)
	}

	return nil
}
