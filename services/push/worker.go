package push

import (
	"fmt"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

// min returns the minimum of two integers
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Worker processes push notification tasks
type Worker struct {
	id      int
	queue   <-chan *NotificationTask
	service *PushService
}

// NewWorker creates a new worker
func NewWorker(id int, queue <-chan *NotificationTask, service *PushService) *Worker {
	return &Worker{
		id:      id,
		queue:   queue,
		service: service,
	}
}

// Start starts the worker
func (w *Worker) Start(wg *sync.WaitGroup) {
	defer wg.Done()

	logging.Infof("Push notification worker %d started", w.id)

	for task := range w.queue {
		w.processTask(task)
	}

	logging.Infof("Push notification worker %d stopped", w.id)
}

// processTask processes a single notification task
func (w *Worker) processTask(task *NotificationTask) {
	task.Attempts++

	logging.Infof("🚀 [Worker %d] Processing push notification - Attempt: %d, Platform: %s, Event Kind: %d, Title: %s",
		w.id, task.Attempts, task.Platform, task.Event.Kind, task.Message.Title)
	logging.Infof("📲 [Worker %d] Device Token: %s..., Recipient: %s",
		w.id, task.DeviceToken[:min(10, len(task.DeviceToken))], task.Pubkey)

	// Log the notification attempt
	log := &types.PushNotificationLog{
		Pubkey:           task.Pubkey,
		EventID:          task.Event.ID,
		EventKind:        task.Event.Kind,
		NotificationType: task.Message.Category,
		DeviceToken:      task.DeviceToken,
		Platform:         task.Platform,
		Delivered:        false,
	}

	// Save log entry
	statsStore := w.service.store.GetStatsStore()
	if statsStore != nil {
		if err := statsStore.LogPushNotification(log); err != nil {
			logging.Errorf("❌ Failed to log push notification to database: %v", err)
		} else {
			logging.Infof("💾 Push notification logged to database with ID: %d", log.ID)
		}
	} else {
		logging.Errorf("❌ Failed to log push notification: Stats store not available")
	}

	// Send the notification
	logging.Infof("📤 [Worker %d] Sending %s push notification to device...", w.id, task.Platform)
	var err error
	switch task.Platform {
	case "ios":
		if w.service.apnsClient != nil {
			err = w.service.apnsClient.SendNotification(task.DeviceToken, task.Message)
		} else {
			err = fmt.Errorf("APNs client not initialized")
		}
	case "android":
		if w.service.fcmClient != nil {
			err = w.service.fcmClient.SendNotification(task.DeviceToken, task.Message)
		} else {
			err = fmt.Errorf("FCM client not initialized")
		}
	default:
		err = fmt.Errorf("unsupported platform: %s", task.Platform)
	}

	// Update log with result
	if err != nil {
		logging.Errorf("❌ [Worker %d] Failed to send push notification - Platform: %s, Error: %v",
			w.id, task.Platform, err)
		logging.Errorf("❌ [Worker %d] Failed notification details - Device: %s..., Event: %s, Kind: %d",
			w.id, task.DeviceToken[:min(10, len(task.DeviceToken))], task.Event.ID, task.Event.Kind)

		// Update log with error
		if log.ID > 0 && statsStore != nil {
			statsStore.UpdatePushNotificationDelivery(log.ID, false, err.Error())
			logging.Infof("📝 Updated notification log ID %d with failure status", log.ID)
		}

		// Retry logic
		if task.Attempts < w.service.config.Service.RetryAttempts {
			// Parse retry delay
			retryDelay, parseErr := time.ParseDuration(w.service.config.Service.RetryDelay)
			if parseErr != nil {
				retryDelay = 5 * time.Second // Default
			}

			// Schedule retry
			go func() {
				time.Sleep(retryDelay)
				select {
				case w.service.queue <- task:
					// Successfully requeued
				case <-w.service.ctx.Done():
					// Service is shutting down
				default:
					logging.Warnf("Failed to requeue notification task, queue full")
				}
			}()
		} else {
			logging.Errorf("🔄 Max retry attempts (%d) reached for notification", w.service.config.Service.RetryAttempts)
			logging.Errorf("🔄 Failed device marked inactive: %s...", task.DeviceToken[:min(10, len(task.DeviceToken))])
			// Mark device as inactive if too many failures
			if statsStore != nil {
				statsStore.UpdatePushDeviceStatus(task.DeviceToken, false)
			}
		}
	} else {
		logging.Infof("✅ [Worker %d] Successfully sent push notification!", w.id)
		logging.Infof("✅ [Worker %d] Success details - Platform: %s, Device: %s..., Event Kind: %d",
			w.id, task.Platform, task.DeviceToken[:min(10, len(task.DeviceToken))], task.Event.Kind)
		logging.Infof("✅ [Worker %d] Notification: %s - %s", w.id, task.Message.Title, task.Message.Body)

		// Update log with success
		if log.ID > 0 && statsStore != nil {
			statsStore.UpdatePushNotificationDelivery(log.ID, true, "")
			logging.Infof("📝 Updated notification log ID %d with success status", log.ID)
		}
	}
}
