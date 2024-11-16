package web

import (
	"encoding/base64"
	"strconv"
	"time"

	"github.com/HORNET-Storage/hornet-storage/lib/stores"
	"github.com/gofiber/fiber/v2"

	types "github.com/HORNET-Storage/hornet-storage/lib"
)

const (
	DefaultPageSize = 10
	MaxPageSize     = 20
)

type FileInfoWithContent struct {
	Hash      string
	FileName  string
	MimeType  string
	Content   string
	Size      int64
	Timestamp time.Time
}

type PaginationMeta struct {
	Page       int   `json:"page"`
	Limit      int   `json:"limit"`
	Total      int64 `json:"total"`
	TotalPages int   `json:"total_pages"`
}

type FilesResponse struct {
	Data []*FileInfoWithContent `json:"data"`
	Meta PaginationMeta         `json:"meta"`
}

func AddContent(fileInfo *types.FileInfo, content []byte) *FileInfoWithContent {
	encodedContent := base64.StdEncoding.EncodeToString(content)

	data := &FileInfoWithContent{
		FileName:  fileInfo.FileName,
		MimeType:  fileInfo.MimeType,
		Content:   encodedContent,
		Size:      fileInfo.Size,
		Timestamp: fileInfo.Timestamp,
	}

	if fileInfo.Root == "blossom" {
		data.Hash = fileInfo.Hash
	} else {
		data.Hash = fileInfo.Root
	}

	return data
}

func HandleGetFilesByType(c *fiber.Ctx, store stores.Store) error {
	mimeType := c.Query("type")
	if mimeType == "" {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"error": "mime type is required",
		})
	}

	page, err := strconv.Atoi(c.Query("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	pageSize, err := strconv.Atoi(c.Query("limit", "10"))
	if err != nil || pageSize < 1 || pageSize > MaxPageSize {
		pageSize = DefaultPageSize
	}

	records, metadata, err := store.GetStatsStore().FetchFilesByType(mimeType, page, pageSize)
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"error": "failed to fetch files",
		})
	}

	files := []*FileInfoWithContent{}

	for _, record := range records {
		if record.Root == "blossom" {
			content, err := store.GetBlob(record.Hash)
			if err != nil {
				data := AddContent(&record, content)

				files = append(files, data)
			}
		} else {
			dag, err := store.BuildDagFromStore(record.Root, true, false)
			if err != nil {
				leaf, ok := dag.Dag.Leafs[record.Hash]
				if ok {
					content, err := dag.Dag.GetContentFromLeaf(leaf)
					if err != nil {
						data := AddContent(&record, content)

						files = append(files, data)
					}
				}
			}
		}
	}

	response := FilesResponse{
		Data: files,
		Meta: PaginationMeta{
			Page:       metadata.CurrentPage,
			Limit:      metadata.PageSize,
			Total:      metadata.TotalItems,
			TotalPages: metadata.TotalPages,
		},
	}

	return c.JSON(response)
}
