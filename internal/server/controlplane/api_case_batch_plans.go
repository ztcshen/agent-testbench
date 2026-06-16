package controlplane

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"

	"agent-testbench/internal/domain/casesuite"
	"agent-testbench/internal/domain/profile"
	"agent-testbench/internal/store"
)

type apiCaseBatchPlanError struct {
	Status  int
	Message string
}

func (e apiCaseBatchPlanError) Error() string {
	return e.Message
}

func apiCaseBatchPlans(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest) ([]apiCaseBatchCasePlan, error) {
	if len(request.CaseIDs) > 0 {
		return apiCaseBatchExactCasePlans(ctx, bundle, runtime, request), nil
	}
	if strings.TrimSpace(request.WorkflowID) != "" {
		return apiCaseBatchWorkflowPlans(ctx, bundle, runtime, request)
	}
	if request.Suite.configured() {
		return apiCaseBatchSuitePlans(ctx, bundle, runtime, request)
	}
	return apiCaseBatchNodePlans(ctx, bundle, runtime, request), nil
}

func apiCaseBatchExactCasePlans(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest) []apiCaseBatchCasePlan {
	casesByID := make(map[string]profile.APICase, len(bundle.APICases))
	for _, item := range bundle.APICases {
		casesByID[item.ID] = item
	}
	cases := make([]profile.APICase, 0, len(request.CaseIDs))
	for _, id := range request.CaseIDs {
		if item, ok := casesByID[id]; ok {
			cases = append(cases, item)
		}
	}
	return apiCaseBatchPlansFromCases(ctx, bundle, runtime, request, cases)
}

func apiCaseBatchNodePlans(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest) []apiCaseBatchCasePlan {
	nodesByID := apiCaseBatchNodesByID(bundle)
	nodeSet := map[string]bool{}
	for _, id := range request.NodeIDs {
		nodeSet[id] = true
	}
	out := make([]apiCaseBatchCasePlan, 0, len(bundle.APICases))
	for _, item := range bundle.APICases {
		if !nodeSet[strings.TrimSpace(item.NodeID)] {
			continue
		}
		plan, ok := apiCaseBatchPlanFromCase(ctx, bundle, runtime, request, item, item.NodeID, nodesByID[item.NodeID], "", map[string]any{"caseId": item.ID})
		if ok {
			out = append(out, plan)
		}
	}
	return out
}

func apiCaseBatchSuitePlans(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest) ([]apiCaseBatchCasePlan, error) {
	filter := casesuite.Filter{
		Filter:   request.Suite.Filter,
		NodeID:   request.Suite.NodeID,
		Tags:     request.Suite.Tags,
		Status:   request.Suite.Status,
		Owner:    request.Suite.Owner,
		Priority: request.Suite.Priority,
	}
	cases := casesuite.SelectCases(bundle, filter)
	if len(request.Suite.RunStates) > 0 {
		report, err := casesuite.Coverage(ctx, bundle, runtime, filter, cases)
		if err != nil {
			return nil, err
		}
		stateSet := casesuite.RunStateSet(request.Suite.RunStates)
		filtered := make([]profile.APICase, 0, len(cases))
		for _, item := range report.Items {
			if !stateSet[casesuite.NormalizeRunState(item.LatestStatus)] {
				continue
			}
			if apiCase, ok := findAPICase(bundle.APICases, item.CaseID); ok {
				filtered = append(filtered, apiCase)
			}
		}
		cases = filtered
	}
	return apiCaseBatchPlansFromCases(ctx, bundle, runtime, request, cases), nil
}

func apiCaseBatchPlansFromCases(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest, cases []profile.APICase) []apiCaseBatchCasePlan {
	nodesByID := apiCaseBatchNodesByID(bundle)
	out := make([]apiCaseBatchCasePlan, 0, len(cases))
	for _, item := range cases {
		plan, ok := apiCaseBatchPlanFromCase(ctx, bundle, runtime, request, item, item.NodeID, nodesByID[item.NodeID], "", map[string]any{"caseId": item.ID})
		if ok {
			out = append(out, plan)
		}
	}
	return out
}

func apiCaseBatchWorkflowPlans(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest) ([]apiCaseBatchCasePlan, error) {
	nodesByID := apiCaseBatchNodesByID(bundle)
	casesByID := make(map[string]profile.APICase, len(bundle.APICases))
	for _, item := range bundle.APICases {
		casesByID[item.ID] = item
	}
	bindings := make([]profile.WorkflowBinding, 0, len(bundle.WorkflowBindings))
	for _, binding := range bundle.WorkflowBindings {
		if binding.WorkflowID == request.WorkflowID {
			bindings = append(bindings, binding)
		}
	}
	sort.SliceStable(bindings, func(i, j int) bool {
		if bindings[i].SortOrder != bindings[j].SortOrder {
			return bindings[i].SortOrder < bindings[j].SortOrder
		}
		return bindings[i].StepID < bindings[j].StepID
	})
	out := make([]apiCaseBatchCasePlan, 0, len(bindings))
	missing := []string{}
	for _, binding := range bindings {
		item, ok := casesByID[binding.CaseID]
		if !ok {
			missing = append(missing, fmt.Sprintf("step %s references missing case %s", binding.StepID, binding.CaseID))
			continue
		}
		nodeID := firstNonEmpty(binding.NodeID, item.NodeID)
		node := nodesByID[nodeID]
		payload := map[string]any{"caseId": item.ID, "workflowId": request.WorkflowID, "stepId": binding.StepID}
		plan, ok := apiCaseBatchPlanFromCase(ctx, bundle, runtime, request, item, nodeID, node, binding.StepID, payload)
		if ok {
			out = append(out, plan)
			continue
		}
		missing = append(missing, fmt.Sprintf("step %s case %s missing runnable case execution", binding.StepID, binding.CaseID))
	}
	if len(missing) > 0 {
		return nil, apiCaseBatchPlanError{
			Status:  http.StatusConflict,
			Message: "workflow " + request.WorkflowID + " has unrunnable steps: " + strings.Join(missing, "; "),
		}
	}
	return out, nil
}

func apiCaseBatchPlanFromCase(ctx context.Context, bundle profile.Bundle, runtime store.Store, request apiCaseBatchRunRequest, item profile.APICase, nodeID string, node profile.InterfaceNode, stepID string, payload map[string]any) (apiCaseBatchCasePlan, bool) {
	casePath := strings.TrimSpace(item.CasePath)
	template := findCaseExecutionTemplateConfig(ctx, runtime, item.ID, payload)
	var execution *caseExecutionConfig
	var exports []map[string]any
	if template != nil {
		execution = &template.CaseExecution
		exports = template.Exports
	}
	if casePath == "" && execution == nil {
		return apiCaseBatchCasePlan{}, false
	}
	return apiCaseBatchCasePlan{
		ID:              item.ID,
		DisplayName:     item.DisplayName,
		Scenario:        item.Scenario,
		NodeID:          nodeID,
		NodeDisplayName: node.DisplayName,
		Operation:       node.Operation,
		Method:          apiCaseBatchPlanMethod(node, execution),
		Path:            apiCaseBatchPlanPath(node, execution),
		StepID:          stepID,
		CasePath:        resolveBatchAPICasePath(ctx, runtime, bundle, casePath),
		BaseURL:         firstNonEmpty(request.BaseURL, item.BaseURL),
		EvidenceDir:     firstNonEmpty(request.EvidenceDir, item.EvidenceDir, filepath.Join(".runtime", "case-batches")),
		TimeoutSeconds:  firstPositive(request.TimeoutSeconds, item.TimeoutSeconds),
		Overrides:       mergeStringAnyMaps(item.DefaultOverrides, request.Overrides),
		Execution:       execution,
		Exports:         exports,
		Case:            item,
	}, true
}

func apiCaseBatchPlanMethod(node profile.InterfaceNode, execution *caseExecutionConfig) string {
	if strings.TrimSpace(node.Method) != "" {
		return node.Method
	}
	if execution != nil {
		return execution.Method
	}
	return ""
}

func apiCaseBatchPlanPath(node profile.InterfaceNode, execution *caseExecutionConfig) string {
	if strings.TrimSpace(node.Path) != "" {
		return node.Path
	}
	if execution != nil {
		return execution.Path
	}
	return ""
}

func resolveBatchAPICasePath(ctx context.Context, runtime store.Store, bundle profile.Bundle, casePath string) string {
	return resolveBundleAPICasePath(ctx, runtime, bundle, casePath)
}

func apiCaseBatchNodesByID(bundle profile.Bundle) map[string]profile.InterfaceNode {
	out := make(map[string]profile.InterfaceNode, len(bundle.InterfaceNodes))
	for _, node := range bundle.InterfaceNodes {
		out[node.ID] = node
	}
	return out
}

func apiCaseBatchNodesFromPlans(plans []apiCaseBatchCasePlan) []apiCaseBatchNodeReport {
	seen := map[string]bool{}
	out := make([]apiCaseBatchNodeReport, 0, len(plans))
	for _, plan := range plans {
		if strings.TrimSpace(plan.NodeID) == "" || seen[plan.NodeID] {
			continue
		}
		seen[plan.NodeID] = true
		out = append(out, apiCaseBatchNodeReport{
			ID:          plan.NodeID,
			DisplayName: plan.NodeDisplayName,
			Operation:   plan.Operation,
			Method:      plan.Method,
			Path:        plan.Path,
		})
	}
	return out
}

func apiCaseBatchReportDir(request apiCaseBatchRunRequest, plans []apiCaseBatchCasePlan) string {
	if strings.TrimSpace(request.EvidenceDir) != "" {
		return request.EvidenceDir
	}
	for _, plan := range plans {
		if strings.TrimSpace(plan.EvidenceDir) != "" {
			return plan.EvidenceDir
		}
	}
	return filepath.Join(".runtime", "case-batches")
}
