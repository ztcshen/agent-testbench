package main

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"agent-testbench/internal/domain/mapplanner"
	"agent-testbench/internal/store"
)

func validateMapRunCatalogPreflight(ctx context.Context, runtime store.Store, graph store.TestPlanGraph, record store.TestMapPlanRecord) error {
	catalog, err := runtime.GetProfileCatalog(ctx)
	if err != nil {
		return fmt.Errorf("map run preflight failed: active catalog unavailable: %w", err)
	}
	check := mapRunCatalogPreflight{
		mapProfileID:        graph.Map.ProfileID,
		activeProfileID:     catalog.ProfileID,
		catalogCaseIDs:      mapRunCatalogCaseIDs(catalog.APICases),
		catalogWorkflowIDs:  mapRunCatalogWorkflowIDs(catalog.Workflows),
		catalogFixtureIDs:   mapRunCatalogFixtureIDs(catalog.Fixtures),
		pathStepsByPath:     mapRunPathStepsByPath(graph.PathSteps),
		materializationByID: mapRunMaterializationsByID(graph.Materializations),
	}
	for _, task := range record.Tasks {
		check.inspectTask(task)
	}
	return check.err()
}

type mapRunCatalogPreflight struct {
	mapProfileID        string
	activeProfileID     string
	catalogCaseIDs      map[string]bool
	catalogWorkflowIDs  map[string]bool
	catalogFixtureIDs   map[string]bool
	pathStepsByPath     map[string][]store.TestPlanPathStep
	materializationByID map[string]store.TestPlanMaterialization
	missingCases        map[string]bool
	missingWorkflows    map[string]bool
	missingFixtures     map[string]bool
}

func (p *mapRunCatalogPreflight) inspectTask(task store.TestMapPlanTask) {
	switch task.Kind {
	case mapplanner.TaskRunPath, mapplanner.TaskRunPathPrefix:
		p.requireWorkflow(task.WorkflowID)
		for _, step := range p.stepsForTask(task) {
			p.requireCase(step.CaseID)
		}
	case mapplanner.TaskRunCase:
		p.requireCase(task.CaseID)
	case mapplanner.TaskReuseMaterialized:
		materialization, ok := p.materializationByID[task.MaterializationID]
		if !ok {
			p.addMissingFixture(task.MaterializationID)
			return
		}
		p.requireWorkflow(materialization.SourceWorkflowID)
		if materialization.FixtureID != "" && !p.catalogFixtureIDs[materialization.FixtureID] {
			p.addMissingFixture(materialization.FixtureID)
		}
	}
}

func (p *mapRunCatalogPreflight) stepsForTask(task store.TestMapPlanTask) []store.TestPlanPathStep {
	steps := append([]store.TestPlanPathStep(nil), p.pathStepsByPath[task.PathID]...)
	untilNodeID := taskUntilNodeID(task)
	if untilNodeID == "" {
		return steps
	}
	for index, step := range steps {
		if step.NodeID == untilNodeID {
			return steps[:index+1]
		}
	}
	return steps
}

func (p *mapRunCatalogPreflight) requireCase(caseID string) {
	caseID = strings.TrimSpace(caseID)
	if caseID == "" || p.catalogCaseIDs[caseID] {
		return
	}
	if p.missingCases == nil {
		p.missingCases = map[string]bool{}
	}
	p.missingCases[caseID] = true
}

func (p *mapRunCatalogPreflight) requireWorkflow(workflowID string) {
	workflowID = strings.TrimSpace(workflowID)
	if workflowID == "" || p.catalogWorkflowIDs[workflowID] {
		return
	}
	if p.missingWorkflows == nil {
		p.missingWorkflows = map[string]bool{}
	}
	p.missingWorkflows[workflowID] = true
}

func (p *mapRunCatalogPreflight) addMissingFixture(fixtureID string) {
	fixtureID = strings.TrimSpace(fixtureID)
	if fixtureID == "" {
		return
	}
	if p.missingFixtures == nil {
		p.missingFixtures = map[string]bool{}
	}
	p.missingFixtures[fixtureID] = true
}

func (p mapRunCatalogPreflight) err() error {
	parts := []string{}
	if strings.TrimSpace(p.mapProfileID) != "" && strings.TrimSpace(p.activeProfileID) != "" && p.mapProfileID != p.activeProfileID {
		parts = append(parts, fmt.Sprintf("active catalog profile %s does not match map profile %s", p.activeProfileID, p.mapProfileID))
	}
	if len(p.missingWorkflows) > 0 {
		parts = append(parts, "missing catalog workflows: "+strings.Join(sortedBoolKeys(p.missingWorkflows), ", "))
	}
	if len(p.missingCases) > 0 {
		parts = append(parts, "missing catalog cases: "+strings.Join(sortedBoolKeys(p.missingCases), ", "))
	}
	if len(p.missingFixtures) > 0 {
		parts = append(parts, "missing catalog fixtures: "+strings.Join(sortedBoolKeys(p.missingFixtures), ", "))
	}
	if len(parts) == 0 {
		return nil
	}
	return fmt.Errorf("map run preflight failed: %s", strings.Join(parts, "; "))
}

func mapRunCatalogCaseIDs(items []store.CatalogAPICase) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			out[id] = true
		}
	}
	return out
}

func mapRunCatalogWorkflowIDs(items []store.CatalogWorkflow) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			out[id] = true
		}
	}
	return out
}

func mapRunCatalogFixtureIDs(items []store.CatalogFixture) map[string]bool {
	out := map[string]bool{}
	for _, item := range items {
		if id := strings.TrimSpace(item.ID); id != "" {
			out[id] = true
		}
	}
	return out
}

func mapRunPathStepsByPath(items []store.TestPlanPathStep) map[string][]store.TestPlanPathStep {
	out := map[string][]store.TestPlanPathStep{}
	for _, item := range items {
		out[item.PathID] = append(out[item.PathID], item)
	}
	for pathID := range out {
		sort.SliceStable(out[pathID], func(i, j int) bool {
			return out[pathID][i].StepIndex < out[pathID][j].StepIndex
		})
	}
	return out
}

func mapRunMaterializationsByID(items []store.TestPlanMaterialization) map[string]store.TestPlanMaterialization {
	out := map[string]store.TestPlanMaterialization{}
	for _, item := range items {
		out[item.ID] = item
	}
	return out
}

func sortedBoolKeys(items map[string]bool) []string {
	out := make([]string, 0, len(items))
	for key := range items {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
