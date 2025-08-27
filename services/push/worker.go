package push

import (
	"fmt"
	"sync"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/types"
)

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
	if err := w.service.store.LogPushNotification(log); err != nil {
		logging.Errorf("Failed to log push notification: %v", err)
	}

	// Send the notification
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
		logging.Errorf("Failed to send push notification to %s (%s): %v", task.DeviceToken, task.Platform, err)

		// Update log with error
		if log.ID > 0 {
			w.service.store.UpdatePushNotificationDelivery(log.ID, false, err.Error())
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
			logging.Errorf("Max retry attempts reached for notification to %s", task.DeviceToken)
			// Mark device as inactive if too many failures
			w.service.store.UpdatePushDeviceStatus(task.DeviceToken, false)
		}
	} else {
		logging.Infof("Successfully sent push notification to %s (%s)", task.DeviceToken, task.Platform)

		// Update log with success
		if log.ID > 0 {
			w.service.store.UpdatePushNotificationDelivery(log.ID, true, "")
		}
	}
}
