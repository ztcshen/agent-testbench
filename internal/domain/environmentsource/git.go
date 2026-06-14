package environmentsource

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func ComponentAssetRemoteRefOK(targetPath string, remoteRefJSON string) bool {
	ref := map[string]any{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(remoteRefJSON)), &ref); err != nil {
		return false
	}
	path := strings.TrimSpace(stringValue(ref["path"]))
	if path == "" {
		path = strings.TrimSpace(targetPath)
	}
	if !relativeAssetPathOK(path) {
		return false
	}
	return IsRemoteGitURL(strings.TrimSpace(stringValue(ref["url"])))
}

func IsRemoteGitURL(rawURL string) bool {
	rawURL = strings.TrimSpace(rawURL)
	lower := strings.ToLower(rawURL)
	for _, prefix := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	at := strings.Index(rawURL, "@")
	colon := strings.Index(rawURL, ":")
	return at > 0 && colon > at+1
}

func relativeAssetPathOK(path string) bool {
	cleanPath := filepath.Clean(path)
	return path != "" && !filepath.IsAbs(path) && cleanPath != "." && cleanPath != ".." && !strings.HasPrefix(cleanPath, ".."+string(os.PathSeparator))
}

func stringValue(value any) string {
	switch item := value.(type) {
	case string:
		return item
	case []byte:
		return string(item)
	case nil:
		return ""
	default:
		return fmt.Sprint(value)
	}
}

func jsonObjectString(raw string) map[string]any {
	out := map[string]any{}
	if err := decodeJSON(raw, "{}", &out); err != nil || out == nil {
		return map[string]any{}
	}
	return out
}

func jsonArrayString(raw string) []any {
	out := []any{}
	if err := decodeJSON(raw, "[]", &out); err != nil || out == nil {
		return []any{}
	}
	return out
}

func decodeJSON(raw string, defaultValue string, target any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		raw = defaultValue
	}
	return json.Unmarshal([]byte(raw), target)
}

func mapFromAny(value any) map[string]any {
	typed, ok := value.(map[string]any)
	if !ok || typed == nil {
		return map[string]any{}
	}
	return typed
}
