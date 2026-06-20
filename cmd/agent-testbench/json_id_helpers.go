package main

import (
	"encoding/json"
	"strings"
)

func jsonID(raw json.RawMessage) string {
	var payload struct {
		ID string `json:"id"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		return ""
	}
	return strings.TrimSpace(payload.ID)
}
