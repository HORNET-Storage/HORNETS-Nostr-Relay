# Content Moderation System

This package provides a content moderation system for Nostr events and media uploads. It's designed to work with the HORNETS-Nostr-Relay system to filter explicit content from the relay.

## Features

- **Multi-Level Moderation**: Uses NudeNet and Llama Vision for comprehensive content analysis
- **Moderation Queue**: Efficiently processes media content with a worker pool
- **Event Filtering**: Analyzes Nostr events for media references and filters based on moderation status
- **Flexible Storage**: Supports GORM-based SQL database storage

## Architecture

The system is composed of the following components:

### Core Components

1. **API Client**: Communicates with the NudeNet/Llama Vision content moderation API
2. **Store**: Persistent storage for moderation records and statuses
3. **Worker Pool**: Processes media in the moderation queue
4. **Service**: Main component that coordinates the moderation system
5. **Reference Analyzer**: Extracts media references from Nostr events

### Data Flow

```
┌───────────┐     ┌───────────┐     ┌───────────┐     ┌───────────┐
│ Uploaded  │     │           │     │           │     │           │
│ Media     │────▶│ Store     │────▶│ Worker    │────▶│ API       │
│           │     │           │     │ Pool      │     │ Client    │
└───────────┘     └───────────┘     └───────────┘     └───────────┘
                        │                 │                 │
                        │                 │                 │
                        ▼                 ▼                 ▼
┌───────────┐     ┌───────────────────────────────────────────────┐
│ Nostr     │     │                                               │
│ Events    │────▶│               Service                         │
│           │     │                                               │
└───────────┘     └───────────────────────────────────────────────┘
```

## Integration with HORNETS-Nostr-Relay

The content moderation system integrates with the existing relay infrastructure through:

1. **Upload Handler**: Intercepts media uploads for moderation
2. **Event Handler**: Filters events containing explicit media
3. **Database Integration**: Uses the existing Statistics database for storage

## Usage

### Initialize the System

```go
import (
    "context"
    "log"
    "time"
    
    "github.com/HORNET-Storage/hornet-storage/lib/contentmoderation"
    "gorm.io/driver/sqlite"
    "gorm.io/gorm"
)

// Initialize the content moderation system
func InitializeContentModeration() {
    // Create a context
    ctx, cancel := context.WithCancel(context.Background())
    defer cancel()
    
    // Load configuration from Viper (or use the default)
    contentmoderation.AddConfigDefaults()
    
    // Create database connection
    db, err := gorm.Open(sqlite.Open("content_moderation.db"), &gorm.Config{})
    if err != nil {
        log.Fatalf("Failed to connect to database: %v", err)
    }
    
    // Initialize the content moderation system
    service, err := contentmoderation.Initialize(ctx, db)
    if err != nil {
        log.Fatalf("Failed to initialize content moderation: %v", err)
    }
    defer contentmoderation.Shutdown()
    
    // Schedule periodic cleanup of rejected media
    contentmoderation.SchedulePeriodicCleanup(ctx, 48*time.Hour)
}
```

### Integrate with Upload Handler

```go
import (
    "log"
    "time"
    
    merkle_dag "github.com/HORNET-Storage/Scionic-Merkle-Tree/dag"
    "github.com/HORNET-Storage/hornet-storage/lib/contentmoderation"
    "github.com/gabriel-vasile/mimetype"
)

// Process an uploaded DAG to check for media content
func processUploadedDag(dag *merkle_dag.Dag, pubKey string) {
    log.Printf("Processing DAG from %s with %d leaves", pubKey, len(dag.Leafs))
    
    // Check for media content in the DAG
    for _, leaf := range dag.Leafs {
        if leaf.Type == "File" {
            // Extract content from leaf
            data, err := dag.GetContentFromLeaf(leaf)
            if err != nil {
                log.Printf("Error extracting content: %v", err)
                continue
            }
            
            // Detect MIME type
            mimeType := mimetype.Detect(data)
            if contentmoderation.IsMediaMimeType(mimeType.String()) {
                log.Printf("Found media content: %s", mimeType.String())
                
                // Create a moderation record
                record := &contentmoderation.ModerationRecord{
                    DagRoot:     dag.Root,
                    ContentType: mimeType.String(),
                    FileSize:    int64(len(data)),
                    UploadedBy:  pubKey,
                    UploadedAt:  time.Now(),
                }
                
                // Queue for moderation
                if err := contentmoderation.GlobalService.EnqueueMedia(record, data); err != nil {
                    log.Printf("Error enqueuing media for moderation: %v", err)
                } else {
                    log.Printf("Enqueued media %s for moderation", dag.Root)
                }
            }
        }
    }
}
```

### Filter Events

```go
import (
    "log"
    
    "github.com/HORNET-Storage/hornet-storage/lib/contentmoderation"
    "github.com/nbd-wtf/go-nostr"
)

// Filter events with explicit content
func FilterExplicitContent(events []*nostr.Event) []*nostr.Event {
    // Make sure the service is initialized
    if contentmoderation.GlobalService == nil {
        log.Println("Content moderation service not initialized!")
        return events
    }
    
    // Filter out events with explicit media
    filteredEvents, err := contentmoderation.FilterEvents(events)
    if err != nil {
        log.Printf("Error filtering events: %v", err)
        return events // Return original events in case of error
    }
    
    log.Printf("Filtered %d events to %d events", len(events), len(filteredEvents))
    return filteredEvents
}
```

### Check Moderation Status

```go
import (
    "github.com/HORNET-Storage/hornet-storage/lib/contentmoderation"
)

// Check if a specific piece of media is approved
func IsMediaApproved(dagRoot string) bool {
    if contentmoderation.GlobalService == nil {
        return false // Conservatively return false if service not initialized
    }
    
    record, err := contentmoderation.GlobalService.GetStore().GetMediaRecord(dagRoot)
    if err != nil {
        return false // If we can't get the record, assume not approved
    }
    
    return record.Status == contentmoderation.StatusApproved
}
```

### Worker Pool Statistics

```go
import (
    "fmt"
    "github.com/HORNET-Storage/hornet-storage/lib/contentmoderation"
)

// Display moderation statistics
func DisplayModerationStats() string {
    if contentmoderation.GlobalService == nil {
        return "Content moderation service not initialized"
    }
    
    stats := contentmoderation.GlobalService.GetWorkerPoolStats()
    
    return fmt.Sprintf(
        "Moderation Statistics:\n" +
        "- Total processed: %d\n" +
        "- Approved: %d\n" +
        "- Rejected: %d\n" +
        "- Processing errors: %d\n" +
        "- Currently processing: %d",
        stats.TotalProcessed,
        stats.TotalApproved,
        stats.TotalRejected,
        stats.ProcessingErrors,
        stats.CurrentlyProcessing,
    )
}
```

## Configuration

The system is configured through the `Config` struct in `config.go`. Configuration can be loaded from YAML or JSON files, or set programmatically.

Key configuration options:

- `APIEndpoint`: URL of the content moderation API
- `APITimeout`: Timeout for API requests
- `NumWorkers`: Number of worker threads for processing
- `TempStoragePath`: Path for temporary storage of media files
- `RetentionPeriod`: How long to keep rejected media before cleanup
- `DefaultMode`: Default moderation mode (basic, strict, or full)

## Database Models

The system uses the following database models:

1. `ModerationRecord`: Tracks media files and their moderation status
2. `EventMediaReference`: Maps events to media references
3. `EventModerationStatus`: Caches event moderation status
4. `ExternalMediaCache`: Caches moderation results for external media

## Moderation Status

Media can have the following status values:

- `StatusAwaiting`: Waiting for moderation
- `StatusProcessing`: Currently being processed
- `StatusApproved`: Approved for delivery
- `StatusRejected`: Rejected due to explicit content
- `StatusDeleted`: Deleted after rejection
