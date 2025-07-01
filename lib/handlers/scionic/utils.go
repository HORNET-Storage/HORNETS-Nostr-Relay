package scionic

import (
	"path/filepath"
	"regexp"
	"strings"

	"github.com/HORNET-Storage/hornet-storage/lib/config"
	"github.com/HORNET-Storage/hornet-storage/lib/types"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
)

type DagWriter func(message interface{}) error

type UploadDagReader func() (*lib_types.UploadMessage, error)
type UploadDagHandler func(read UploadDagReader, write DagWriter)

type DownloadDagReader func() (*lib_types.DownloadMessage, error)
type DownloadDagHandler func(read DownloadDagReader, write DagWriter)

type QueryDagReader func() (*lib_types.QueryMessage, error)
type QueryDagHandler func(read QueryDagReader, write DagWriter)

func IsMimeTypePermitted(mimeType string) bool {
	settings, err := config.GetConfig()
	if err != nil {
		return false
	}

	// Only check if media
	if len(settings.EventFiltering.MediaDefinitions) > 0 {
		return checkMimeTypeInDefinitions(mimeType, settings.EventFiltering.MediaDefinitions)
	}

	// Default: allow if no restrictions are configured
	return true
}

// IsFilePermitted checks both MIME type and file extension
func IsFilePermitted(filename, mimeType string) bool {
	settings, err := config.GetConfig()
	if err != nil {
		return false
	}

	// Check against new MediaDefinitions
	if settings.EventFiltering.MediaDefinitions != nil {
		return checkFileInDefinitions(filename, mimeType, settings.EventFiltering.MediaDefinitions)
	}

	// Fallback to legacy check
	return IsMimeTypePermitted(mimeType)
}

// checkMimeTypeInDefinitions checks if MIME type matches any enabled media definition
func checkMimeTypeInDefinitions(mimeType string, definitions map[string]types.MediaDefinition) bool {
	for _, definition := range definitions {
		// Check MIME patterns
		for _, pattern := range definition.MimePatterns {
			if matchesMimePattern(mimeType, pattern) {
				return true
			}
		}
	}
	return false
}

// checkFileInDefinitions checks both extension and MIME type against enabled definitions
func checkFileInDefinitions(filename, mimeType string, definitions map[string]types.MediaDefinition) bool {
	ext := strings.ToLower(filepath.Ext(filename))

	for _, definition := range definitions {
		// Check extensions first (faster)
		for _, allowedExt := range definition.Extensions {
			if ext == strings.ToLower(allowedExt) {
				return true
			}
		}

		// Check MIME patterns
		for _, pattern := range definition.MimePatterns {
			if matchesMimePattern(mimeType, pattern) {
				return true
			}
		}
	}
	return false
}

// matchesMimePattern checks if a MIME type matches a pattern (supports wildcards)
func matchesMimePattern(mimeType, pattern string) bool {
	// Direct match
	if mimeType == pattern {
		return true
	}

	// Wildcard pattern matching (e.g., "image/*", "video/*")
	if strings.Contains(pattern, "*") {
		// Convert glob pattern to regex
		regexPattern := strings.ReplaceAll(pattern, "*", ".*")
		regexPattern = "^" + regexPattern + "$"

		if regex, err := regexp.Compile(regexPattern); err == nil {
			return regex.MatchString(mimeType)
		}
	}

	return false
}

// GetMediaTypeInfo returns information about a file's media type
func GetMediaTypeInfo(filename, mimeType string) (mediaType string, definition *types.MediaDefinition) {
	settings, err := config.GetConfig()
	if err != nil {
		return "unknown", nil
	}

	if settings.EventFiltering.MediaDefinitions == nil {
		return "unknown", nil
	}

	ext := strings.ToLower(filepath.Ext(filename))

	// Find matching media type
	for typeName, def := range settings.EventFiltering.MediaDefinitions {
		// Check extensions
		for _, allowedExt := range def.Extensions {
			if ext == strings.ToLower(allowedExt) {
				return typeName, &def
			}
		}

		// Check MIME patterns
		for _, pattern := range def.MimePatterns {
			if matchesMimePattern(mimeType, pattern) {
				return typeName, &def
			}
		}
	}

	return "unknown", nil
}
