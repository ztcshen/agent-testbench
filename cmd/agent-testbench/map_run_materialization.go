package main

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"agent-testbench/internal/store"
)

func (e mapRunExecutor) executeMaterializedTask(task *store.TestMapPlanTask) {
	materialization, ok := e.matByID[task.MaterializationID]
	if !ok {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": "materialization not found: " + task.MaterializationID}, time.Now().UTC())
		return
	}
	fixture, ok, err := e.catalogFixture(materialization.FixtureID)
	if err != nil {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": err.Error()}, time.Now().UTC())
		return
	}
	if !ok {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": "fixture not found: " + materialization.FixtureID}, time.Now().UTC())
		return
	}
	overrides, err := mapRunMaterializedOverrides(fixture.DataJSON, e.materializationMappings(materialization.ID, fixture.ID))
	if err != nil {
		e.finishTask(task, store.StatusFailed, map[string]any{"error": err.Error()}, time.Now().UTC())
		return
	}
	e.exportsByTask[task.ID] = overrides
	e.finishTask(task, store.StatusPassed, map[string]any{
		"materializationId": materialization.ID,
		"fixtureId":         fixture.ID,
		"sourcePathId":      materialization.SourcePathID,
		"sourceWorkflowId":  materialization.SourceWorkflowID,
		"sourceUntilNodeId": materialization.SourceUntilNodeID,
		"overrides":         mapRunSortedKeys(overrides),
		"exports":           overrides,
	}, time.Now().UTC())
}

func (e mapRunExecutor) catalogFixture(fixtureID string) (store.CatalogFixture, bool, error) {
	catalog, err := e.runtime.GetProfileCatalog(e.ctx)
	if err != nil {
		return store.CatalogFixture{}, false, err
	}
	for _, fixture := range catalog.Fixtures {
		if fixture.ID != fixtureID {
			continue
		}
		if strings.TrimSpace(fixture.Status) != "" && fixture.Status != "active" {
			return store.CatalogFixture{}, false, fmt.Errorf("fixture is not active: %s", fixture.ID)
		}
		return fixture, true, nil
	}
	return store.CatalogFixture{}, false, nil
}

func (e mapRunExecutor) materializationMappings(materializationID string, fixtureID string) []map[string]any {
	mappings := []map[string]any{}
	targetCases := e.materializationTargetCaseIDs(materializationID)
	for _, edge := range e.graph.Edges {
		if edge.MaterializationID == materializationID && edge.Required {
			mappings = append(mappings, listOfMaps(edge.MappingsJSON)...)
		}
	}
	catalog, err := e.runtime.GetProfileCatalog(e.ctx)
	if err != nil {
		return mappings
	}
	for _, dependency := range catalog.CaseDependencies {
		if dependency.FixtureID != fixtureID || !dependency.Required {
			continue
		}
		if strings.TrimSpace(dependency.Status) != "" && dependency.Status != "active" {
			continue
		}
		if len(targetCases) > 0 && !targetCases[dependency.CaseID] {
			continue
		}
		mappings = append(mappings, listOfMaps(dependency.MappingsJSON)...)
	}
	return mappings
}

func (e mapRunExecutor) materializationTargetCaseIDs(materializationID string) map[string]bool {
	targets := map[string]bool{}
	for _, edge := range e.graph.Edges {
		if edge.MaterializationID != materializationID || !edge.Required {
			continue
		}
		node := e.nodeByID[edge.ToNodeID]
		caseID := firstNonEmpty(node.CaseID, edge.ToNodeID)
		if strings.TrimSpace(caseID) != "" {
			targets[caseID] = true
		}
	}
	return targets
}

func mapRunMaterializedOverrides(rawData string, mappings []map[string]any) (map[string]any, error) {
	data := map[string]any{}
	if strings.TrimSpace(rawData) != "" {
		if err := json.Unmarshal([]byte(rawData), &data); err != nil {
			return nil, fmt.Errorf("decode materialized fixture data: %w", err)
		}
	}
	out := mapRunCopyStringAnyMap(data)
	for _, mapping := range mappings {
		value := mapRunJSONPathValue(data, valueString(mapping["from"]))
		if value == nil {
			continue
		}
		for _, key := range mapRunOverrideKeys(valueString(mapping["to"])) {
			out[key] = value
		}
	}
	return out, nil
}

func mapRunJSONPathValue(root map[string]any, path string) any {
	path = strings.TrimSpace(path)
	path = strings.TrimPrefix(path, "$.")
	path = strings.TrimPrefix(path, "$")
	if path == "" {
		return root
	}
	return workflowValueAtPath(root, path)
}

func mapRunOverrideKeys(path string) []string {
	trimmed := strings.TrimSpace(path)
	trimmed = strings.TrimPrefix(trimmed, "$.")
	trimmed = strings.TrimPrefix(trimmed, "$")
	if trimmed == "" {
		return nil
	}
	keys := []string{trimmed}
	parts := strings.Split(trimmed, ".")
	leaf := parts[len(parts)-1]
	if leaf != "" && leaf != trimmed {
		keys = append(keys, leaf)
	}
	return keys
}

func listOfMaps(raw string) []map[string]any {
	var items []map[string]any
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil
	}
	return items
}

func mapRunSortedKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func mapRunCopyStringAnyMap(in map[string]any) map[string]any {
	out := map[string]any{}
	for key, value := range in {
		out[key] = value
	}
	return out
}
