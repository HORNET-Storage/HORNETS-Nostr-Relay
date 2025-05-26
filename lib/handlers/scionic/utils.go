package scionic

import (
	"github.com/spf13/viper"

	lib_types "github.com/HORNET-Storage/go-hornet-storage-lib/lib"
	types "github.com/HORNET-Storage/hornet-storage/lib"
)

type DagWriter func(message interface{}) error

type UploadDagReader func() (*lib_types.UploadMessage, error)
type UploadDagHandler func(read UploadDagReader, write DagWriter)

type DownloadDagReader func() (*lib_types.DownloadMessage, error)
type DownloadDagHandler func(read DownloadDagReader, write DagWriter)

type QueryDagReader func() (*lib_types.QueryMessage, error)
type QueryDagHandler func(read QueryDagReader, write DagWriter)

func IsMimeTypePermitted(mimeType string) bool {
	settings, err := RetrieveSettings()
	if err != nil {
		return false
	}

	if len(settings.MimeTypeWhitelist) > 0 {
		if !contains(settings.MimeTypeWhitelist, mimeType) {
			return false
		}
	}

	return true
}

func RetrieveSettings() (*types.RelaySettings, error) {
	var settings types.RelaySettings
	if err := viper.UnmarshalKey("relay_settings", &settings); err != nil {
		return nil, err
	}

	return &settings, nil
}

func contains(list []string, item string) bool {
	for _, element := range list {
		if element == item {
			return true
		}
	}
	return false
}
