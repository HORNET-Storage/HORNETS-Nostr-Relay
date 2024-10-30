package web

import (
	"encoding/hex"
	"fmt"
	"log"
	"strconv"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"
)

// Define the supported media file types
var audioFileTypes = []string{"mp3", "wav", "ogg", "flac", "aac", "wma", "m4a", "opus", "m4b", "midi", "mp4", "webm", "3gp"}
var photoFileTypes = []string{"jpeg", "jpg", "png", "gif", "bmp", "tiff", "raw", "svg", "eps", "psd", "ai", "pdf", "webp"}
var videoFileTypes = []string{"avi", "mp4", "mov", "wmv", "mkv", "flv", "mpeg", "3gp", "webm"}

type MediaResponse struct {
	Items      []MediaItem `json:"items"`
	TotalCount int         `json:"totalCount"`
	NextCursor string      `json:"nextCursor,omitempty"`
}

type MediaItem struct {
	Hash        string            `json:"hash"`
	Name        string            `json:"name"`
	Type        string            `json:"type"`
	ContentHash []byte            `json:"contentHash,omitempty"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func GetMedia(c *fiber.Ctx, store stores.Store) error {
	log.Println("Getting Media.")

	// Parse query parameters
	mediaType := c.Query("type", "all")                    // Default to all types
	pageSize, _ := strconv.Atoi(c.Query("pageSize", "20")) // Limit the page size
	cursor := c.Query("cursor", "")                        // For pagination

	// Validate pageSize
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	// Retrieve the list of cache buckets to use for querying the DAG
	cacheBuckets, err := store.GetMasterBucketList("cache")
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": fmt.Sprintf("Failed to retrieve cache bucket list: %v", err),
		})
	}
	log.Printf("Cache buckets: %v", cacheBuckets)

	// Combine all media types when "all" is selected
	var mediaFileTypes []string
	if mediaType == "all" {
		// If media type is "all", we combine all file types into one list
		mediaFileTypes = append(mediaFileTypes, audioFileTypes...)
		mediaFileTypes = append(mediaFileTypes, photoFileTypes...)
		mediaFileTypes = append(mediaFileTypes, videoFileTypes...)
	} else {
		// Otherwise, filter by specific media type
		switch mediaType {
		case "audio":
			mediaFileTypes = audioFileTypes
		case "image", "photo":
			mediaFileTypes = photoFileTypes
		case "video":
			mediaFileTypes = videoFileTypes
		default:
			return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
				"error": "Unsupported media type",
			})
		}
	}

	// Initialize to collect all the keys found
	var allKeys []string

	// Query the DAG for each cache bucket and for each media file type
	for _, bucket := range cacheBuckets {
		// Extract the user/app key from the cache bucket (remove the "cache:" prefix)
		bucketKey := strings.TrimPrefix(bucket, "cache:")

		// Query for each media file type
		for _, fileType := range mediaFileTypes {
			log.Printf("Processing bucket: %s with key %s", bucketKey, fileType)
			bucketFilter := map[string]string{bucketKey: fileType}
			keys, err := store.QueryDag(bucketFilter)
			if err != nil {
				return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
					"error": fmt.Sprintf("Failed to query media: %v", err),
				})
			}

			// Accumulate keys across all queries
			allKeys = append(allKeys, keys...)
		}
	}
	log.Printf("Keys found: %v", allKeys)

	// Initialize the response
	response := MediaResponse{
		Items: make([]MediaItem, 0),
	}

	// Handle pagination using the cursor
	startIndex := 0
	if cursor != "" {
		for i, key := range allKeys {
			if key == cursor {
				startIndex = i + 1
				break
			}
		}
	}

	// Get paginated results
	endIndex := startIndex + pageSize
	if endIndex > len(allKeys) {
		endIndex = len(allKeys)
	}

	// Set next cursor if there are more items
	if endIndex < len(allKeys) {
		response.NextCursor = allKeys[endIndex-1]
	}

	// Retrieve each media item from the DAG
	for _, key := range allKeys[startIndex:endIndex] {
		leafData, err := store.RetrieveLeaf(key, key, false) // Do not include content initially
		if err != nil {
			continue // Skip any items that can't be retrieved
		}

		// Filter by file extension
		fileExtension := strings.ToLower(getFileExtension(leafData.Leaf.ItemName))
		if !isSupportedFileType(fileExtension, mediaFileTypes) {
			continue // Skip unsupported file types
		}

		// Create a media item from the leaf data
		item := MediaItem{
			Hash:        leafData.Leaf.Hash,
			Name:        leafData.Leaf.ItemName,
			Type:        fileExtension,
			ContentHash: leafData.Leaf.ContentHash,
			Metadata:    leafData.Leaf.AdditionalData,
		}

		response.Items = append(response.Items, item)
	}

	response.TotalCount = len(allKeys)
	return c.JSON(response)
}

func GetMediaContent(c *fiber.Ctx, store stores.Store) error {
	hash := c.Params("hash")
	if hash == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "Hash parameter is required",
		})
	}

	// Retrieve the leaf data with the content
	leafData, err := store.RetrieveLeaf(hash, hash, true) // Include content in the response
	if err != nil {
		return c.Status(fiber.StatusNotFound).JSON(fiber.Map{
			"error": fmt.Sprintf("Content not found: %v", err),
		})
	}

	// Set the content type based on the file extension
	contentType := getContentType(leafData.Leaf.ItemName)
	c.Set("Content-Type", contentType)

	log.Println("Leaf Data Content: ", hex.EncodeToString(leafData.Leaf.Content))

	// Send the content as the response
	return c.Send(leafData.Leaf.Content)
}

// Helper function to extract file extension
func getFileExtension(filename string) string {
	parts := strings.Split(filename, ".")
	if len(parts) > 1 {
		return parts[len(parts)-1]
	}
	return ""
}

// Check if the file type is supported
func isSupportedFileType(fileExtension string, supportedFileTypes []string) bool {
	for _, fileType := range supportedFileTypes {
		if fileType == fileExtension {
			return true
		}
	}
	return false
}

// Updated getContentType function
func getContentType(filename string) string {
	// Extract the file extension (assuming extensions are case-insensitive)
	extension := strings.ToLower(strings.TrimPrefix(strings.ToLower(filename[strings.LastIndex(filename, ".")+1:]), "."))

	// Check if the extension belongs to an image, video, or audio
	if isInList(extension, photoFileTypes) {
		return getImageContentType(extension)
	} else if isInList(extension, videoFileTypes) {
		return getVideoContentType(extension)
	} else if isInList(extension, audioFileTypes) {
		return getAudioContentType(extension)
	}

	// Default content type for unsupported file types
	return "application/octet-stream"
}

// Helper function to check if a file extension is in a given list
func isInList(extension string, list []string) bool {
	for _, item := range list {
		if item == extension {
			return true
		}
	}
	return false
}

// Helper function to return the correct image content type
func getImageContentType(extension string) string {
	switch extension {
	case "jpeg", "jpg":
		return "image/jpeg"
	case "png":
		return "image/png"
	case "gif":
		return "image/gif"
	case "bmp":
		return "image/bmp"
	case "tiff":
		return "image/tiff"
	case "webp":
		return "image/webp"
	case "svg":
		return "image/svg+xml"
	case "pdf":
		return "application/pdf"
	case "eps":
		return "application/postscript"
	default:
		return "application/octet-stream" // For unsupported image formats
	}
}

// Helper function to return the correct video content type
func getVideoContentType(extension string) string {
	switch extension {
	case "mp4":
		return "video/mp4"
	case "mov":
		return "video/quicktime"
	case "wmv":
		return "video/x-ms-wmv"
	case "mkv":
		return "video/x-matroska"
	case "flv":
		return "video/x-flv"
	case "avi":
		return "video/x-msvideo"
	case "mpeg":
		return "video/mpeg"
	case "3gp":
		return "video/3gpp"
	case "webm":
		return "video/webm"
	default:
		return "application/octet-stream" // For unsupported video formats
	}
}

// Helper function to return the correct audio content type
func getAudioContentType(extension string) string {
	switch extension {
	case "mp3":
		return "audio/mpeg"
	case "wav":
		return "audio/wav"
	case "ogg":
		return "audio/ogg"
	case "flac":
		return "audio/flac"
	case "aac":
		return "audio/aac"
	case "wma":
		return "audio/x-ms-wma"
	case "m4a":
		return "audio/mp4"
	case "opus":
		return "audio/opus"
	case "m4b":
		return "audio/x-m4b"
	case "midi":
		return "audio/midi"
	case "webm":
		return "audio/webm"
	case "3gp":
		return "audio/3gpp"
	default:
		return "application/octet-stream" // For unsupported audio formats
	}
}
