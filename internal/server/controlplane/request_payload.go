package controlplane

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

func readJSONPayload(r *http.Request) (payload map[string]any, err error) {
	defer func() {
		if closeErr := r.Body.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		raw = []byte("{}")
	}
	var decoded map[string]any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if err := decoder.Decode(&decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func valueString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case json.Number:
		return typed.String()
	default:
		return fmt.Sprint(value)
	}
}

func intFromAny(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	case json.Number:
		out, err := typed.Int64()
		if err != nil {
			return 0
		}
		return int(out)
	case string:
		out, err := strconv.Atoi(strings.TrimSpace(typed))
		if err != nil {
			return 0
		}
		return out
	default:
		return 0
	}
}
