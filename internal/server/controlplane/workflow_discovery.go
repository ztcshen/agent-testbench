package controlplane

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strings"

	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

func handleWorkflowDiscovery(w http.ResponseWriter, r *http.Request, bundle profile.Bundle, runtime store.Store) {
	payload, err := WorkflowDiscoveryPayload(r.Context(), bundle, r.URL.Query().Get("filter"), runtime)
	if err != nil {
		writeJSONStatus(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, payload)
}

func WorkflowDiscoveryPayload(ctx context.Context, bundle profile.Bundle, filter string, runtime store.Store) (map[string]any, error) {
	if runtime != nil {
		catalog, err := runtime.GetProfileCatalog(ctx)
		if err != nil && !errors.Is(err, store.ErrNotFound) {
			return nil, err
		}
		if err == nil && len(catalog.Workflows) > 0 {
			return workflowDiscoveryPayloadFromCatalog(catalog, filter), nil
		}
	}
	return workflowDiscoveryPayloadFromBundle(bundle, filter), nil
}

type workflowDiscoverySource struct {
	ID          string
	DisplayName string
	Description string
}

func workflowDiscoveryPayloadFromBundle(bundle profile.Bundle, filter string) map[string]any {
	workflows := make([]workflowDiscoverySource, 0, len(bundle.Workflows))
	for _, workflow := range bundle.Workflows {
		workflows = append(workflows, workflowDiscoverySource{
			ID:          workflow.ID,
			DisplayName: workflow.DisplayName,
			Description: workflow.Description,
		})
	}
	return workflowDiscoveryPayload(bundle.ID, workflowDiscoveryItems(workflows, workflowStepCounts(bundle.WorkflowBindings), filter), "profile")
}

func workflowDiscoveryPayloadFromCatalog(catalog store.ProfileCatalog, filter string) map[string]any {
	workflows := make([]workflowDiscoverySource, 0, len(catalog.Workflows))
	for _, workflow := range catalog.Workflows {
		workflows = append(workflows, workflowDiscoverySource{
			ID:          workflow.ID,
			DisplayName: workflow.DisplayName,
			Description: workflow.Description,
		})
	}
	return workflowDiscoveryPayload(catalog.ProfileID, workflowDiscoveryItems(workflows, catalogWorkflowStepCounts(catalog.WorkflowBindings), filter), "store")
}

func workflowDiscoveryItems(workflows []workflowDiscoverySource, stepCounts map[string]int, filter string) []map[string]any {
	sort.SliceStable(workflows, func(i, j int) bool {
		return workflows[i].ID < workflows[j].ID
	})
	items := make([]map[string]any, 0, len(workflows))
	for _, workflow := range workflows {
		if !matchesControlplaneDiscoveryFilter(filter, workflow.ID, workflow.DisplayName, workflow.Description) {
			continue
		}
		items = append(items, map[string]any{
			"id":          workflow.ID,
			"displayName": workflow.DisplayName,
			"description": workflow.Description,
			"stepCount":   stepCounts[workflow.ID],
		})
	}
	return items
}

func workflowStepCounts(bindings []profile.WorkflowBinding) map[string]int {
	stepCounts := make(map[string]int, len(bindings))
	for _, binding := range bindings {
		addWorkflowStepCount(stepCounts, binding.WorkflowID)
	}
	return stepCounts
}

func catalogWorkflowStepCounts(bindings []store.CatalogWorkflowBinding) map[string]int {
	stepCounts := make(map[string]int, len(bindings))
	for _, binding := range bindings {
		addWorkflowStepCount(stepCounts, binding.WorkflowID)
	}
	return stepCounts
}

func addWorkflowStepCount(stepCounts map[string]int, workflowID string) {
	if strings.TrimSpace(workflowID) != "" {
		stepCounts[workflowID]++
	}
}

func workflowDiscoveryPayload(profileID string, items []map[string]any, sourceKind string) map[string]any {
	return map[string]any{
		"ok":        true,
		"profileId": profileID,
		"count":     len(items),
		"items":     items,
		"source":    map[string]any{"kind": sourceKind},
	}
}

func matchesControlplaneDiscoveryFilter(filter string, values ...string) bool {
	filter = strings.ToLower(strings.TrimSpace(filter))
	if filter == "" {
		return true
	}
	for _, value := range values {
		if strings.Contains(strings.ToLower(value), filter) {
			return true
		}
	}
	return false
}
