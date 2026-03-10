package telegram

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mymmrac/telego"

	"github.com/nextlevelbuilder/goclaw/internal/channels/media"
)

const (
	// defaultMediaMaxBytes is the default max download size (20MB, Telegram Bot API limit).
	defaultMediaMaxBytes int64 = 20 * 1024 * 1024

	// downloadMaxRetries is the number of download retry attempts.
	downloadMaxRetries = 3
)

// MediaInfo is an alias for the shared media.MediaInfo type.
type MediaInfo = media.MediaInfo

// resolveMedia extracts and downloads media from a Telegram message.
// Returns a list of MediaInfo for each media item found.
func (c *Channel) resolveMedia(ctx context.Context, msg *telego.Message) []MediaInfo {
	var results []MediaInfo

	maxBytes := c.config.MediaMaxBytes
	if maxBytes == 0 {
		maxBytes = defaultMediaMaxBytes
	}

	// Photo: take highest resolution (last element)
	if msg.Photo != nil && len(msg.Photo) > 0 {
		photo := msg.Photo[len(msg.Photo)-1]
		filePath, err := c.downloadMedia(ctx, photo.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download photo", "file_id", photo.FileID, "error", err)
		} else {
			// Pass raw file to agent loop — sanitization now happens at loop level.
			results = append(results, MediaInfo{
				Type:        "image",
				FilePath:    filePath,
				FileID:      photo.FileID,
				ContentType: "image/jpeg",
				FileSize:    int64(photo.FileSize),
			})
		}
	}

	// Video
	if msg.Video != nil {
		filePath, err := c.downloadMedia(ctx, msg.Video.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download video", "file_id", msg.Video.FileID, "error", err)
		} else {
			results = append(results, MediaInfo{
				Type:        "video",
				FilePath:    filePath,
				FileID:      msg.Video.FileID,
				ContentType: msg.Video.MimeType,
				FileName:    msg.Video.FileName,
				FileSize:    int64(msg.Video.FileSize),
			})
		}
	}

	// Video Note (round video)
	if msg.VideoNote != nil {
		filePath, err := c.downloadMedia(ctx, msg.VideoNote.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download video note", "file_id", msg.VideoNote.FileID, "error", err)
		} else {
			results = append(results, MediaInfo{
				Type:        "video",
				FilePath:    filePath,
				FileID:      msg.VideoNote.FileID,
				ContentType: "video/mp4",
				FileSize:    int64(msg.VideoNote.FileSize),
			})
		}
	}

	// Animation (GIF)
	if msg.Animation != nil {
		filePath, err := c.downloadMedia(ctx, msg.Animation.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download animation", "file_id", msg.Animation.FileID, "error", err)
		} else {
			results = append(results, MediaInfo{
				Type:        "animation",
				FilePath:    filePath,
				FileID:      msg.Animation.FileID,
				ContentType: msg.Animation.MimeType,
				FileName:    msg.Animation.FileName,
				FileSize:    int64(msg.Animation.FileSize),
			})
		}
	}

	// Audio
	if msg.Audio != nil {
		filePath, err := c.downloadMedia(ctx, msg.Audio.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download audio", "file_id", msg.Audio.FileID, "error", err)
		} else {
			results = append(results, MediaInfo{
				Type:        "audio",
				FilePath:    filePath,
				FileID:      msg.Audio.FileID,
				ContentType: msg.Audio.MimeType,
				FileName:    msg.Audio.FileName,
				FileSize:    int64(msg.Audio.FileSize),
			})
		}
	}

	// Voice
	if msg.Voice != nil {
		filePath, err := c.downloadMedia(ctx, msg.Voice.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download voice", "file_id", msg.Voice.FileID, "error", err)
		} else {
			results = append(results, MediaInfo{
				Type:        "voice",
				FilePath:    filePath,
				FileID:      msg.Voice.FileID,
				ContentType: msg.Voice.MimeType,
				FileSize:    int64(msg.Voice.FileSize),
			})
		}
	}

	// Document
	if msg.Document != nil {
		filePath, err := c.downloadMedia(ctx, msg.Document.FileID, maxBytes)
		if err != nil {
			slog.Warn("failed to download document", "file_id", msg.Document.FileID, "error", err)
		} else {
			results = append(results, MediaInfo{
				Type:        "document",
				FilePath:    filePath,
				FileID:      msg.Document.FileID,
				ContentType: msg.Document.MimeType,
				FileName:    msg.Document.FileName,
				FileSize:    int64(msg.Document.FileSize),
			})
		}
	}

	return results
}

// downloadMedia downloads a file from Telegram by file_id with retry logic.
// Returns the local file path.
func (c *Channel) downloadMedia(ctx context.Context, fileID string, maxBytes int64) (string, error) {
	var file *telego.File
	var err error

	// Retry up to downloadMaxRetries times with exponential backoff
	for attempt := 1; attempt <= downloadMaxRetries; attempt++ {
		file, err = c.bot.GetFile(ctx, &telego.GetFileParams{FileID: fileID})
		if err == nil {
			break
		}
		if attempt < downloadMaxRetries {
			slog.Debug("retrying file download", "file_id", fileID, "attempt", attempt, "error", err)
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(attempt) * time.Second):
			}
		}
	}
	if err != nil {
		return "", fmt.Errorf("get file info after %d attempts: %w", downloadMaxRetries, err)
	}

	if file.FilePath == "" {
		return "", fmt.Errorf("empty file path for file_id %s", fileID)
	}

	// Check file size before downloading
	if int64(file.FileSize) > maxBytes {
		return "", fmt.Errorf("file too large: %d bytes (max %d)", file.FileSize, maxBytes)
	}

	// Build download URL
	downloadURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", c.config.Token, file.FilePath)

	resp, err := http.Get(downloadURL)
	if err != nil {
		return "", fmt.Errorf("download file: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download failed with status %d", resp.StatusCode)
	}

	// Determine extension from file path
	ext := filepath.Ext(file.FilePath)
	if ext == "" {
		ext = ".bin"
	}

	tmpFile, err := os.CreateTemp("", "goclaw_media_*"+ext)
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	// Copy with size limit
	written, err := io.Copy(tmpFile, io.LimitReader(resp.Body, maxBytes+1))
	if err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("save file: %w", err)
	}
	if written > maxBytes {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("file exceeds max size during download: %d bytes", written)
	}

	return tmpFile.Name(), nil
}

// buildMediaTags delegates to the shared media package.
func buildMediaTags(mediaList []MediaInfo) string {
	return media.BuildMediaTags(mediaList)
}

// extractDocumentContent delegates to the shared media package.
func extractDocumentContent(filePath, fileName string) (string, error) {
	return media.ExtractDocumentContent(filePath, fileName)
}

// lightweightMediaTags builds media placeholder tags from Telegram message metadata
// without downloading any files. Used for pending history recording when bot is not mentioned.
func lightweightMediaTags(msg *telego.Message) string {
	var tags []string
	if msg.Photo != nil && len(msg.Photo) > 0 {
		tags = append(tags, "<media:image>")
	}
	if msg.Video != nil {
		tags = append(tags, "<media:video>")
	}
	if msg.VideoNote != nil {
		tags = append(tags, "<media:video>")
	}
	if msg.Animation != nil {
		tags = append(tags, "<media:video>")
	}
	if msg.Audio != nil {
		tags = append(tags, "<media:audio>")
	}
	if msg.Voice != nil {
		tags = append(tags, "<media:voice>")
	}
	if msg.Document != nil {
		name := msg.Document.FileName
		if name != "" {
			tags = append(tags, fmt.Sprintf("<media:document name=%q>", name))
		} else {
			tags = append(tags, "<media:document>")
		}
	}
	if len(tags) == 0 {
		return ""
	}
	return strings.Join(tags, "\n")
}
