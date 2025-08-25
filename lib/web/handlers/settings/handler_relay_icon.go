package settings

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/gabriel-vasile/mimetype"
	"github.com/gofiber/fiber/v2"
	"github.com/nbd-wtf/go-nostr"
	"github.com/spf13/viper"

	"github.com/HORNET-Storage/go-hornet-storage-lib/lib/signing"
	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/logging"
	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/HORNET-Storage/hornet-storage/lib/subscription"
)

// UploadRelayIcon handles uploading and setting the relay icon
func UploadRelayIcon(c *fiber.Ctx, store stores.Store) error {
	logging.Info("Relay icon upload request received")

	// Get panel URL from form data
	panelURL := c.FormValue("panel_url")
	if panelURL == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "panel_url is required",
		})
	}

	// Get image data from form file
	file, err := c.FormFile("image")
	if err != nil {
		logging.Infof("Error getting image file: %v", err)
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "image file is required",
		})
	}

	// Open the uploaded file
	fileData, err := file.Open()
	if err != nil {
		logging.Infof("Error opening uploaded file: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to process uploaded file",
		})
	}
	defer fileData.Close()

	// Read file data
	data := make([]byte, file.Size)
	if _, err := fileData.Read(data); err != nil {
		logging.Infof("Error reading file data: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to read uploaded file",
		})
	}

	// Validate file size (5MB limit)
	const maxFileSize = 5 * 1024 * 1024 // 5MB
	if len(data) > maxFileSize {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "file size exceeds 5MB limit",
		})
	}

	// Validate MIME type (only images)
	mtype := mimetype.Detect(data)
	if !strings.HasPrefix(mtype.String(), "image/") {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "only image files are allowed",
		})
	}

	// Generate SHA-256 hash
	checkHash := sha256.Sum256(data)
	encodedHash := hex.EncodeToString(checkHash[:])

	logging.Infof("Processing relay icon upload - Hash: %s, Type: %s, Size: %d", encodedHash, mtype.String(), len(data))

	// Get relay private key for creating Kind 117 event
	serializedPrivateKey := viper.GetString("relay.private_key")
	if len(serializedPrivateKey) <= 0 {
		logging.Error("No private key found in configuration")
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "relay configuration error",
		})
	}

	privateKey, publicKey, err := signing.DeserializePrivateKey(serializedPrivateKey)
	if err != nil {
		logging.Infof("Error deserializing private key: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "relay configuration error",
		})
	}

	// Create Kind 117 event for the relay icon
	publicKeyHex := hex.EncodeToString(publicKey.SerializeCompressed())
	event := &nostr.Event{
		Kind:      117,
		PubKey:    publicKeyHex,
		CreatedAt: nostr.Now(),
		Tags: nostr.Tags{
			{"blossom_hash", encodedHash},
			{"name", "relay-icon"},
			{"type", mtype.String()},
			{"size", fmt.Sprintf("%d", len(data))},
		},
		Content: "Relay icon uploaded via admin panel",
	}

	// Sign the event
	if err := event.Sign(hex.EncodeToString(privateKey.Serialize())); err != nil {
		logging.Infof("Error signing Kind 117 event: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to create upload authorization",
		})
	}

	// Store the Kind 117 event
	if err := store.StoreEvent(event); err != nil {
		logging.Infof("Error storing Kind 117 event: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to store upload authorization",
		})
	}

	// Store the blob in Blossom
	if err := store.StoreBlob(data, checkHash[:], publicKeyHex); err != nil {
		logging.Infof("Error storing blob: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to store image",
		})
	}

	// Store file statistics
	store.GetStatsStore().SaveFile("blossom", encodedHash, "relay-icon", mtype.String(), 0, int64(len(data)))

	// Update subscription storage usage
	go func(pk string, size int64) {
		subManager := subscription.GetGlobalManager()
		if subManager != nil {
			if err := subManager.UpdateStorageUsage(pk, size); err != nil {
				logging.Infof("Warning: Failed to update storage usage for pubkey %s: %v", pk, err)
			}
		}
	}(publicKeyHex, int64(len(data)))

	// Construct the full Blossom URL
	panelURL = strings.TrimSuffix(panelURL, "/")
	blossomURL := fmt.Sprintf("%s/blossom/%s", panelURL, encodedHash)

	// Update relay icon in config
	// Use UpdateConfig with save=true since this is a legitimate config update
	if err := config.UpdateConfig("relay.icon", blossomURL, true); err != nil {
		logging.Infof("Warning: Failed to update relay icon in config: %v", err)
		// Don't fail the request, just log the warning
	}

	logging.Infof("Successfully uploaded relay icon: %s", blossomURL)

	return c.JSON(fiber.Map{
		"success": true,
		"message": "Relay icon uploaded successfully",
		"hash":    encodedHash,
		"url":     blossomURL,
		"size":    len(data),
		"type":    mtype.String(),
	})
}
