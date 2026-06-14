package controlplane

import (
	"net/http"
	"strings"

	"agent-testbench/internal/domain/apicasecommand"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
	"agent-testbench/internal/store/apicaserunstate"
)

func handleCaseIncompleteBatches(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store) {
	if runtime == nil {
		writeJSON(w, map[string]any{
			"ok":       true,
			"count":    0,
			"items":    []map[string]any{},
			"warnings": []string{"runtime store is not configured"},
		})
		return
	}
	passed, latest, err := apicaserunstate.StatusByCase(r.Context(), runtime, bundle.ID)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}

	items := make([]map[string]any, 0)
	for _, item := range bundle.APICases {
		if strings.TrimSpace(item.ID) == "" || passed[item.ID] {
			continue
		}
		reason := "not-run"
		if status := latest[item.ID]; status != "" {
			reason = "latest-" + status
		}
		items = append(items, map[string]any{
			"id":               item.ID,
			"title":            firstNonEmpty(item.DisplayName, item.ID),
			"reason":           reason,
			"source":           "profile:" + bundle.ID,
			"message":          "no passed Store run found for this API Case",
			"suggestedCommand": apicasecommand.SuggestedRunCommandForProfile(item, bundle.ID),
		})
	}
	writeJSON(w, map[string]any{
		"ok":       true,
		"count":    len(items),
		"items":    items,
		"warnings": []string{},
	})
}
