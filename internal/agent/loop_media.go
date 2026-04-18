package agent

import (
	"os"
	"path/filepath"
	"strings"
)

// parseMediaResult extracts a MediaResult from a tool result string containing "MEDIA:" prefix.
// Handles formats: "MEDIA:/path/to/file" and "[[audio_as_voice]]\nMEDIA:/path/to/file".
// Returns nil if no MEDIA: prefix is found.
//
// IMPORTANT: Only matches "MEDIA:" at the start of the (trimmed) string to avoid false
// positives when tool output contains "MEDIA:" in arbitrary text (e.g. a web page
// mentioning a commit message like "return MEDIA: path from screenshot").
func parseMediaResult(toolOutput string) *MediaResult {
	s := toolOutput
	asVoice := false

	// Check for [[audio_as_voice]] tag (TTS voice messages)
	if strings.Contains(s, "[[audio_as_voice]]") {
		asVoice = true
		s = strings.ReplaceAll(s, "[[audio_as_voice]]", "")
	}

	s = strings.TrimSpace(s)

	// Only match MEDIA: at the beginning of the string.
	if !strings.HasPrefix(s, "MEDIA:") {
		return nil
	}
	path := strings.TrimSpace(s[6:])
	if path == "" {
		return nil
	}
	// Take only the first line (in case there's trailing text)
	if nl := strings.IndexByte(path, '\n'); nl >= 0 {
		path = strings.TrimSpace(path[:nl])
	}

	return &MediaResult{
		Path:        path,
		ContentType: mimeFromExt(filepath.Ext(path)),
		AsVoice:     asVoice,
	}
}

// extractMediaFromContent scans text for MEDIA:<path> tokens the LLM may echo
// in its final response (e.g. when a tool returned the MEDIA: prefix as plain
// text instead of setting Result.Media). Relative paths are resolved against
// workspace. Called before sanitize strips the tokens so the attachments are
// still delivered.
//
// Security: only paths that (a) exist on disk and (b) resolve inside the
// workspace root are accepted. An LLM cannot inject attachments pointing at
// /etc/passwd, a sibling tenant's workspace, or a hallucinated path — the
// extractor silently drops them. When workspace is empty, only legacy absolute
// paths from tool outputs (via parseMediaResult's upstream flow) are trusted;
// LLM-echoed absolute paths without a workspace context are dropped.
func extractMediaFromContent(content, workspace string) []MediaResult {
	if !strings.Contains(content, "MEDIA:") || workspace == "" {
		return nil
	}
	matches := mediaPathPattern.FindAllString(content, -1)
	if len(matches) == 0 {
		return nil
	}
	wsRoot := ""
	if abs, err := filepath.Abs(workspace); err == nil {
		wsRoot = filepath.Clean(abs)
	}
	if wsRoot == "" {
		return nil
	}
	results := make([]MediaResult, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, m := range matches {
		path := strings.TrimSpace(strings.TrimPrefix(m, "MEDIA:"))
		if path == "" {
			continue
		}
		// Drop markdown/JSON trailing punctuation that would otherwise stick:
		// ")", "]", "\"", "'", ",", ";", ".".
		path = strings.TrimRight(path, `)]"',;.`)
		if path == "" {
			continue
		}
		// Resolve relative paths against workspace.
		if !filepath.IsAbs(path) {
			path = filepath.Join(wsRoot, path)
		}
		cleaned := filepath.Clean(path)
		// Workspace containment check — blocks "../" escapes and absolute paths
		// outside the agent's allowed area.
		rel, err := filepath.Rel(wsRoot, cleaned)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		// Existence + regular-file check prevents hallucinated paths and
		// directory/symlink-device shenanigans from reaching the outbound queue.
		info, err := os.Lstat(cleaned)
		if err != nil || !info.Mode().IsRegular() {
			continue
		}
		if _, dup := seen[cleaned]; dup {
			continue
		}
		seen[cleaned] = struct{}{}
		results = append(results, MediaResult{
			Path:        cleaned,
			ContentType: mimeFromExt(filepath.Ext(cleaned)),
		})
	}
	return results
}

// deduplicateMedia removes duplicate media results by path, keeping the first occurrence.
func deduplicateMedia(media []MediaResult) []MediaResult {
	if len(media) <= 1 {
		return media
	}
	seen := make(map[string]bool, len(media))
	result := make([]MediaResult, 0, len(media))
	for _, m := range media {
		if seen[m.Path] {
			continue
		}
		seen[m.Path] = true
		result = append(result, m)
	}
	return result
}

// mimeFromExt returns a MIME type for common media file extensions.
func mimeFromExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".png":
		return "image/png"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".gif":
		return "image/gif"
	case ".webp":
		return "image/webp"
	case ".mp4":
		return "video/mp4"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".mp3":
		return "audio/mpeg"
	case ".wav":
		return "audio/wav"
	case ".txt":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".csv":
		return "text/csv"
	case ".json":
		return "application/json"
	case ".html", ".htm":
		return "text/html"
	case ".xml":
		return "application/xml"
	case ".zip":
		return "application/zip"
	case ".doc", ".docx":
		return "application/msword"
	case ".xls", ".xlsx":
		return "application/vnd.ms-excel"
	case ".md":
		return "text/markdown"
	default:
		return "application/octet-stream"
	}
}
