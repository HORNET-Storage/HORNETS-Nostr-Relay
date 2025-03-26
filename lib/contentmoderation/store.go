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

// Store defines the interface for content moderation storage operations
type Store interface {
	// Media moderation operations
	EnqueueMedia(record *ModerationRecord, data []byte) error
	GetMediaRecord(dagRoot string) (*ModerationRecord, error)
	GetNextAwaitingMedia() (*ModerationRecord, []byte, error)
	UpdateMediaRecord(record *ModerationRecord) error
	DeleteRejectedMedia(dagRoot string) error

	// Event reference operations
	SaveEventMediaReference(reference *EventMediaReference) error
	GetEventMediaReferences(eventID string) ([]MediaReferenceInfo, error)
	MarkEventStatus(eventID string, status EventStatus) error
	GetEventStatus(eventID string) (EventStatus, error)

	// External media operations
	SaveExternalMediaCache(url string, status ContentStatus, contentType string, fileHash string) error
	GetExternalMediaCache(url string) (*ExternalMediaCache, error)

	// Maintenance operations
	CleanupExpiredRecords(cutoff time.Time) error

	// Initialization and cleanup
	Initialize() error
	Close() error
}

// DBStore implements the Store interface using a GORM database
type DBStore struct {
	db        *gorm.DB
	config    *Config
	tempDir   string
	cacheLock sync.RWMutex
	cache     map[string]*ModerationRecord // Simple in-memory cache
}

// NewDBStore creates a new database store for content moderation
func NewDBStore(db *gorm.DB, config *Config) (*DBStore, error) {
	// Ensure temp directory exists
	if err := os.MkdirAll(config.TempStoragePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	store := &DBStore{
		db:      db,
		config:  config,
		tempDir: config.TempStoragePath,
		cache:   make(map[string]*ModerationRecord),
	}

	return store, nil
}

// Initialize sets up the database tables and indexes
func (s *DBStore) Initialize() error {
	// Auto migrate the tables
	if err := s.db.AutoMigrate(
		&ModerationRecord{},
		&EventMediaReference{},
		&EventModerationStatus{},
		&ExternalMediaCache{},
	); err != nil {
		return fmt.Errorf("failed to migrate tables: %w", err)
	}

	return nil
}

// Close cleans up any resources
func (s *DBStore) Close() error {
	return nil
}

// EnqueueMedia adds a new media item to the moderation queue and stores its data
func (s *DBStore) EnqueueMedia(record *ModerationRecord, data []byte) error {
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

	// Save to database
	if err := s.db.Create(record).Error; err != nil {
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

// GetMediaRecord retrieves a moderation record by DAG root
func (s *DBStore) GetMediaRecord(dagRoot string) (*ModerationRecord, error) {
	// Check cache first
	s.cacheLock.RLock()
	if record, ok := s.cache[dagRoot]; ok {
		s.cacheLock.RUnlock()
		return record, nil
	}
	s.cacheLock.RUnlock()

	// Not in cache, query database
	var record ModerationRecord
	if err := s.db.Where("dag_root = ?", dagRoot).First(&record).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrMediaNotFound
		}
		return nil, fmt.Errorf("failed to get record: %w", err)
	}

	// Add to cache
	s.cacheLock.Lock()
	s.cache[dagRoot] = &record
	s.cacheLock.Unlock()

	return &record, nil
}

// GetNextAwaitingMedia gets the next media item waiting for moderation
func (s *DBStore) GetNextAwaitingMedia() (*ModerationRecord, []byte, error) {
	var record ModerationRecord

	// Start a transaction for safe status update
	tx := s.db.Begin()
	if tx.Error != nil {
		return nil, nil, fmt.Errorf("failed to begin transaction: %w", tx.Error)
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// Find the next awaiting record
	if err := tx.Where("status = ?", StatusAwaiting).
		Order("created_at ASC").
		First(&record).Error; err != nil {
		tx.Rollback()
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrNoMediaWaiting
		}
		return nil, nil, fmt.Errorf("failed to get awaiting record: %w", err)
	}

	// Update status to processing
	record.Status = StatusProcessing
	record.AttemptCount++
	record.UpdatedAt = time.Now()
	if err := tx.Save(&record).Error; err != nil {
		tx.Rollback()
		return nil, nil, fmt.Errorf("failed to update record status: %w", err)
	}

	// Commit the transaction
	if err := tx.Commit().Error; err != nil {
		return nil, nil, fmt.Errorf("failed to commit transaction: %w", err)
	}

	// Update cache
	s.cacheLock.Lock()
	s.cache[record.DagRoot] = &record
	s.cacheLock.Unlock()

	// Get the file data
	filePath := GenerateTempFilePath(record.DagRoot, record.ContentType, s.config)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return &record, nil, fmt.Errorf("failed to read file data: %w", err)
	}

	if s.config.Debug {
		log.Printf("Retrieved next awaiting media: %s", record.DagRoot)
	}

	return &record, data, nil
}

// UpdateMediaRecord updates a moderation record
func (s *DBStore) UpdateMediaRecord(record *ModerationRecord) error {
	if record == nil {
		return errors.New("nil record")
	}

	// Update timestamp
	record.UpdatedAt = time.Now()

	// Update in database
	if err := s.db.Save(record).Error; err != nil {
		return fmt.Errorf("failed to update record: %w", err)
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

// DeleteRejectedMedia deletes the file data for rejected media
func (s *DBStore) DeleteRejectedMedia(dagRoot string) error {
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

// SaveEventMediaReference saves a reference between an event and media
func (s *DBStore) SaveEventMediaReference(reference *EventMediaReference) error {
	if reference == nil {
		return errors.New("nil reference")
	}

	// Save to database
	if err := s.db.Create(reference).Error; err != nil {
		return fmt.Errorf("failed to save event media reference: %w", err)
	}

	return nil
}

// GetEventMediaReferences gets all media references for an event
func (s *DBStore) GetEventMediaReferences(eventID string) ([]MediaReferenceInfo, error) {
	var references []EventMediaReference
	var result []MediaReferenceInfo

	// Get references from database
	if err := s.db.Where("event_id = ?", eventID).Find(&references).Error; err != nil {
		return nil, fmt.Errorf("failed to get event media references: %w", err)
	}

	// Convert to MediaReferenceInfo
	for _, ref := range references {
		info := MediaReferenceInfo{
			MediaURL:   ref.MediaURL,
			MediaID:    ref.MediaID,
			SourceType: ref.SourceType,
			Type:       "event_reference",
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
func (s *DBStore) MarkEventStatus(eventID string, status EventStatus) error {
	// Create or update status
	now := time.Now()
	eventStatus := EventModerationStatus{
		EventID:   eventID,
		Status:    status,
		CheckedAt: now,
	}

	// Upsert in database
	if err := s.db.Save(&eventStatus).Error; err != nil {
		return fmt.Errorf("failed to update event status: %w", err)
	}

	return nil
}

// GetEventStatus gets the moderation status for an event
func (s *DBStore) GetEventStatus(eventID string) (EventStatus, error) {
	var status EventModerationStatus

	// Get from database
	if err := s.db.Where("event_id = ?", eventID).First(&status).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", fmt.Errorf("event status not found")
		}
		return "", fmt.Errorf("failed to get event status: %w", err)
	}

	return status.Status, nil
}

// SaveExternalMediaCache saves a cache entry for external media
func (s *DBStore) SaveExternalMediaCache(url string, status ContentStatus, contentType string, fileHash string) error {
	// Create cache entry
	now := time.Now()
	cache := ExternalMediaCache{
		URL:         url,
		Status:      status,
		CheckedAt:   now,
		FileHash:    fileHash,
		ContentType: contentType,
	}

	// Upsert in database
	if err := s.db.Save(&cache).Error; err != nil {
		return fmt.Errorf("failed to save external media cache: %w", err)
	}

	return nil
}

// GetExternalMediaCache gets a cache entry for external media
func (s *DBStore) GetExternalMediaCache(url string) (*ExternalMediaCache, error) {
	var cache ExternalMediaCache

	// Get from database
	if err := s.db.Where("url = ?", url).First(&cache).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("external media cache not found")
		}
		return nil, fmt.Errorf("failed to get external media cache: %w", err)
	}

	// Check if expired
	if time.Since(cache.CheckedAt) > s.config.CacheTTL {
		return nil, fmt.Errorf("external media cache expired")
	}

	return &cache, nil
}

// CleanupExpiredRecords removes old records and temporary files
func (s *DBStore) CleanupExpiredRecords(cutoff time.Time) error {
	// Find rejected records older than cutoff
	var records []ModerationRecord
	if err := s.db.Where("status = ? AND updated_at < ?", StatusRejected, cutoff).Find(&records).Error; err != nil {
		return fmt.Errorf("failed to find expired records: %w", err)
	}

	// Delete each record's file
	for _, record := range records {
		filePath := GenerateTempFilePath(record.DagRoot, record.ContentType, s.config)
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			log.Printf("Error deleting expired file %s: %v", filePath, err)
		}

		// Update status to deleted
		record.Status = StatusDeleted
		if err := s.db.Save(&record).Error; err != nil {
			log.Printf("Error updating record %s status: %v", record.DagRoot, err)
		}

		// Remove from cache
		s.cacheLock.Lock()
		delete(s.cache, record.DagRoot)
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
			s.db.Model(&ModerationRecord{}).Where("dag_root = ?", dagRoot).Count(&count)
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
	if err := s.db.Where("checked_at < ?", cutoff).Delete(&ExternalMediaCache{}).Error; err != nil {
		log.Printf("Error cleaning up external media cache: %v", err)
	}

	return nil
}

// InMemoryStore is a simple in-memory implementation of the Store interface
// for testing or simple deployments without a database
type InMemoryStore struct {
	config   *Config
	tempDir  string
	lock     sync.RWMutex
	records  map[string]*ModerationRecord
	refs     map[string][]EventMediaReference
	statuses map[string]EventModerationStatus
	extCache map[string]ExternalMediaCache
}

// NewInMemoryStore creates a new in-memory store
func NewInMemoryStore(config *Config) (*InMemoryStore, error) {
	// Ensure temp directory exists
	if err := os.MkdirAll(config.TempStoragePath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create temp directory: %w", err)
	}

	return &InMemoryStore{
		config:   config,
		tempDir:  config.TempStoragePath,
		records:  make(map[string]*ModerationRecord),
		refs:     make(map[string][]EventMediaReference),
		statuses: make(map[string]EventModerationStatus),
		extCache: make(map[string]ExternalMediaCache),
	}, nil
}

// Initialize sets up the in-memory store
func (s *InMemoryStore) Initialize() error {
	return nil
}

// Close cleans up any resources
func (s *InMemoryStore) Close() error {
	return nil
}

// EnqueueMedia adds a new media item to the moderation queue and stores its data
func (s *InMemoryStore) EnqueueMedia(record *ModerationRecord, data []byte) error {
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

	// Store in memory
	s.lock.Lock()
	s.records[record.DagRoot] = record
	s.lock.Unlock()

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

// GetMediaRecord retrieves a moderation record by DAG root
func (s *InMemoryStore) GetMediaRecord(dagRoot string) (*ModerationRecord, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	record, ok := s.records[dagRoot]
	if !ok {
		return nil, ErrMediaNotFound
	}

	return record, nil
}

// GetNextAwaitingMedia gets the next media item waiting for moderation
func (s *InMemoryStore) GetNextAwaitingMedia() (*ModerationRecord, []byte, error) {
	s.lock.Lock()
	defer s.lock.Unlock()

	// Find the oldest awaiting record
	var oldestRecord *ModerationRecord
	var oldestTime time.Time

	for _, record := range s.records {
		if record.Status == StatusAwaiting {
			if oldestRecord == nil || record.CreatedAt.Before(oldestTime) {
				oldestRecord = record
				oldestTime = record.CreatedAt
			}
		}
	}

	if oldestRecord == nil {
		return nil, nil, ErrNoMediaWaiting
	}

	// Update status to processing
	oldestRecord.Status = StatusProcessing
	oldestRecord.AttemptCount++
	oldestRecord.UpdatedAt = time.Now()

	// Get the file data
	filePath := GenerateTempFilePath(oldestRecord.DagRoot, oldestRecord.ContentType, s.config)
	data, err := os.ReadFile(filePath)
	if err != nil {
		return oldestRecord, nil, fmt.Errorf("failed to read file data: %w", err)
	}

	return oldestRecord, data, nil
}

// UpdateMediaRecord updates a moderation record
func (s *InMemoryStore) UpdateMediaRecord(record *ModerationRecord) error {
	if record == nil {
		return errors.New("nil record")
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	// Update timestamp
	record.UpdatedAt = time.Now()

	// Update in memory
	s.records[record.DagRoot] = record

	return nil
}

// DeleteRejectedMedia deletes the file data for rejected media
func (s *InMemoryStore) DeleteRejectedMedia(dagRoot string) error {
	s.lock.Lock()
	record, ok := s.records[dagRoot]
	s.lock.Unlock()

	if !ok {
		return ErrMediaNotFound
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

	// Update status
	s.lock.Lock()
	record.Status = StatusDeleted
	s.lock.Unlock()

	return nil
}

// SaveEventMediaReference saves a reference between an event and media
func (s *InMemoryStore) SaveEventMediaReference(reference *EventMediaReference) error {
	if reference == nil {
		return errors.New("nil reference")
	}

	s.lock.Lock()
	defer s.lock.Unlock()

	s.refs[reference.EventID] = append(s.refs[reference.EventID], *reference)

	return nil
}

// GetEventMediaReferences gets all media references for an event
func (s *InMemoryStore) GetEventMediaReferences(eventID string) ([]MediaReferenceInfo, error) {
	s.lock.RLock()
	refs := s.refs[eventID]
	s.lock.RUnlock()

	var result []MediaReferenceInfo

	for _, ref := range refs {
		info := MediaReferenceInfo{
			MediaURL:   ref.MediaURL,
			MediaID:    ref.MediaID,
			SourceType: ref.SourceType,
			Type:       "event_reference",
		}

		// If this is a scionic reference, add moderation status
		if ref.SourceType == "scionic" {
			s.lock.RLock()
			record, ok := s.records[ref.MediaID]
			s.lock.RUnlock()

			if ok {
				info.IsModerated = true
				info.Status = record.Status
			}
		}

		result = append(result, info)
	}

	return result, nil
}

// MarkEventStatus sets the moderation status for an event
func (s *InMemoryStore) MarkEventStatus(eventID string, status EventStatus) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.statuses[eventID] = EventModerationStatus{
		EventID:   eventID,
		Status:    status,
		CheckedAt: time.Now(),
	}

	return nil
}

// GetEventStatus gets the moderation status for an event
func (s *InMemoryStore) GetEventStatus(eventID string) (EventStatus, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	status, ok := s.statuses[eventID]
	if !ok {
		return "", fmt.Errorf("event status not found")
	}

	return status.Status, nil
}

// SaveExternalMediaCache saves a cache entry for external media
func (s *InMemoryStore) SaveExternalMediaCache(url string, status ContentStatus, contentType string, fileHash string) error {
	s.lock.Lock()
	defer s.lock.Unlock()

	s.extCache[url] = ExternalMediaCache{
		URL:         url,
		Status:      status,
		CheckedAt:   time.Now(),
		FileHash:    fileHash,
		ContentType: contentType,
	}

	return nil
}

// GetExternalMediaCache gets a cache entry for external media
func (s *InMemoryStore) GetExternalMediaCache(url string) (*ExternalMediaCache, error) {
	s.lock.RLock()
	defer s.lock.RUnlock()

	cache, ok := s.extCache[url]
	if !ok {
		return nil, fmt.Errorf("external media cache not found")
	}

	// Check if expired
	if time.Since(cache.CheckedAt) > s.config.CacheTTL {
		return nil, fmt.Errorf("external media cache expired")
	}

	return &cache, nil
}

// CleanupExpiredRecords removes old records and temporary files
func (s *InMemoryStore) CleanupExpiredRecords(cutoff time.Time) error {
	// This is a simplified implementation for the in-memory store
	// A real implementation would clean up old records and files

	// Clean up external media cache
	s.lock.Lock()
	for url, cache := range s.extCache {
		if cache.CheckedAt.Before(cutoff) {
			delete(s.extCache, url)
		}
	}
	s.lock.Unlock()

	return nil
}
